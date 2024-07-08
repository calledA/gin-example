// Copyright 2014 Manu Martinez-Almeida. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package gin

import (
	"errors"
	"io"
	"log"
	"math"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-contrib/sse"
	"github.com/gin-gonic/gin/binding"
	"github.com/gin-gonic/gin/render"
)

// 常见的Content-Type类型，在binding中声明
const (
	MIMEJSON              = binding.MIMEJSON
	MIMEHTML              = binding.MIMEHTML
	MIMEXML               = binding.MIMEXML
	MIMEXML2              = binding.MIMEXML2
	MIMEPlain             = binding.MIMEPlain
	MIMEPOSTForm          = binding.MIMEPOSTForm
	MIMEMultipartPOSTForm = binding.MIMEMultipartPOSTForm
	MIMEYAML              = binding.MIMEYAML
	MIMETOML              = binding.MIMETOML
)

// BodyBytesKey indicates a default body bytes key.
const BodyBytesKey = "_gin-gonic/gin/bodybyteskey"

// ContextKey is the key that a Context returns itself for.
const ContextKey = "_gin-gonic/gin/contextkey"

// abortIndex represents a typical value used in abort functions.
const abortIndex int8 = math.MaxInt8 >> 1

// Context是gin中最重要的部分，可以通过Context在middleware中传递变量，请求链路控制、校验JSON参数以及response的JSON render
type Context struct {
	writermem responseWriter
	Request   *http.Request
	Writer    ResponseWriter

	Params   Params
	handlers HandlersChain
	index    int8
	fullPath string

	engine       *Engine
	params       *Params
	skippedNodes *[]skippedNode

	// This mutex protects Keys map.
	mu sync.RWMutex

	// Keys is a key/value pair exclusively for the context of each request.
	Keys map[string]any

	// Errors is a list of errors attached to all the handlers/middlewares who used this context.
	Errors errorMsgs

	// Accepted defines a list of manually accepted formats for content negotiation.
	Accepted []string

	// queryCache caches the query result from c.Request.URL.Query().
	queryCache url.Values

	// formCache caches c.Request.PostForm, which contains the parsed form data from POST, PATCH,
	// or PUT body parameters.
	formCache url.Values

	// SameSite allows a server to define a cookie attribute making it impossible for
	// the browser to send this cookie along with cross-site requests.
	sameSite http.SameSite
}

/************************************/
/********** CONTEXT CREATION ********/
/************************************/

// 重置Context
func (c *Context) reset() {
	c.Writer = &c.writermem
	c.Params = c.Params[:0]
	c.handlers = nil
	c.index = -1

	c.fullPath = ""
	c.Keys = nil
	c.Errors = c.Errors[:0]
	c.Accepted = nil
	c.queryCache = nil
	c.formCache = nil
	c.sameSite = 0
	*c.params = (*c.params)[:0]
	*c.skippedNodes = (*c.skippedNodes)[:0]
}

// 返回当前Context的copy（safe），仅当需要把context传入goroutine时使用
func (c *Context) Copy() *Context {
	cp := Context{
		writermem: c.writermem,
		Request:   c.Request,
		Params:    c.Params,
		engine:    c.engine,
	}
	cp.writermem.ResponseWriter = nil
	cp.Writer = &cp.writermem
	cp.index = abortIndex
	cp.handlers = nil
	cp.Keys = map[string]any{}
	for k, v := range c.Keys {
		cp.Keys[k] = v
	}
	paramCopy := make([]Param, len(cp.Params))
	copy(paramCopy, cp.Params)
	cp.Params = paramCopy
	return &cp
}

// 返回mian的handler's name，eg：handleGetUsers()会返回main.handleGetUsers
func (c *Context) HandlerName() string {
	return nameOfFunction(c.handlers.Last())
}

// 返回所有的handler's name
func (c *Context) HandlerNames() []string {
	hn := make([]string, 0, len(c.handlers))
	for _, val := range c.handlers {
		hn = append(hn, nameOfFunction(val))
	}
	return hn
}

// 返回mian的handler
func (c *Context) Handler() HandlerFunc {
	return c.handlers.Last()
}

// 返回匹配到的route全路径，没匹配到则返回空字符串
//
//	router.GET("/user/:id", func(c *gin.Context) {
//	    c.FullPath() == "/user/:id" // true
//	})
func (c *Context) FullPath() string {
	return c.fullPath
}

/************************************/
/*********** FLOW CONTROL ***********/
/************************************/

// 在中间件内部使用，执行下一个handler
func (c *Context) Next() {
	c.index++
	for c.index < int8(len(c.handlers)) {
		c.handlers[c.index](c)
		c.index++
	}
}

// 当前Context aborted之后，返回true
func (c *Context) IsAborted() bool {
	return c.index >= abortIndex
}

// 调用Abort停止请求链路，防止调用待处理程序，但不会停止当前处理程序
// eg：假设有个授权中间件，如果授权失败，调用Abort，可以防止调用此请求的其他处理程序
func (c *Context) Abort() {
	c.index = abortIndex
}

// 调用Abort停止请求链路之前写入status，eg：授权失败返回context.AbortWithStatus(401)
func (c *Context) AbortWithStatus(code int) {
	c.Status(code)
	c.Writer.WriteHeaderNow()
	c.Abort()
}

// 调用Abort停止请求链路，之后使用传入的jsonObj返回json对象，设置Content-Type为application/json
func (c *Context) AbortWithStatusJSON(code int, jsonObj any) {
	c.Abort()
	c.JSON(code, jsonObj)
}

// 调用AbortWithStatus停止请求链路，之后写入c.Error，使用部分在Context.Error()
func (c *Context) AbortWithError(code int, err error) *Error {
	c.AbortWithStatus(code)
	return c.Error(err)
}

/************************************/
/********* ERROR MANAGEMENT *********/
/************************************/

// 将Error添加到Context，同时将Error添加到errorMsgs中，方便后续使用
func (c *Context) Error(err error) *Error {
	// error如果为空，调用Error会panic
	if err == nil {
		panic("err is nil")
	}

	var parsedError *Error
	ok := errors.As(err, &parsedError)
	// 如果是Error类型，进行类型转换处理
	if !ok {
		parsedError = &Error{
			Err:  err,
			Type: ErrorTypePrivate,
		}
	}

	c.Errors = append(c.Errors, parsedError)
	return parsedError
}

/************************************/
/******** METADATA MANAGEMENT********/
/************************************/

// 为Context存储新的key/value键值对，使用懒加载初始化c.Keys
func (c *Context) Set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Keys == nil {
		c.Keys = make(map[string]any)
	}

	c.Keys[key] = value
}

// 获取指定的key
func (c *Context) Get(key string) (value any, exists bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	value, exists = c.Keys[key]
	return
}

// 获取指定的key，如果不存在则会panic
func (c *Context) MustGet(key string) any {
	if value, exists := c.Get(key); exists {
		return value
	}
	panic("Key \"" + key + "\" does not exist")
}

// 获取指定的key，结果返回string类型
func (c *Context) GetString(key string) (s string) {
	if val, ok := c.Get(key); ok && val != nil {
		s, _ = val.(string)
	}
	return
}

// 获取指定的key，结果返回bool类型
func (c *Context) GetBool(key string) (b bool) {
	if val, ok := c.Get(key); ok && val != nil {
		b, _ = val.(bool)
	}
	return
}

// 获取指定的key，结果返回int类型
func (c *Context) GetInt(key string) (i int) {
	if val, ok := c.Get(key); ok && val != nil {
		i, _ = val.(int)
	}
	return
}

// 获取指定的key，结果返回int64类型
func (c *Context) GetInt64(key string) (i64 int64) {
	if val, ok := c.Get(key); ok && val != nil {
		i64, _ = val.(int64)
	}
	return
}

// 获取指定的key，结果返回uint类型
func (c *Context) GetUint(key string) (ui uint) {
	if val, ok := c.Get(key); ok && val != nil {
		ui, _ = val.(uint)
	}
	return
}

// 获取指定的key，结果返回uint64类型
func (c *Context) GetUint64(key string) (ui64 uint64) {
	if val, ok := c.Get(key); ok && val != nil {
		ui64, _ = val.(uint64)
	}
	return
}

// 获取指定的key，结果返回float64类型
func (c *Context) GetFloat64(key string) (f64 float64) {
	if val, ok := c.Get(key); ok && val != nil {
		f64, _ = val.(float64)
	}
	return
}

// 获取指定的key，结果返回time.Time类型
func (c *Context) GetTime(key string) (t time.Time) {
	if val, ok := c.Get(key); ok && val != nil {
		t, _ = val.(time.Time)
	}
	return
}

// 获取指定的key，结果返回time.Duration类型
func (c *Context) GetDuration(key string) (d time.Duration) {
	if val, ok := c.Get(key); ok && val != nil {
		d, _ = val.(time.Duration)
	}
	return
}

// 获取指定的key，结果返回[]string类型
func (c *Context) GetStringSlice(key string) (ss []string) {
	if val, ok := c.Get(key); ok && val != nil {
		ss, _ = val.([]string)
	}
	return
}

// 获取指定的key，结果返回map[string]any类型
func (c *Context) GetStringMap(key string) (sm map[string]any) {
	if val, ok := c.Get(key); ok && val != nil {
		sm, _ = val.(map[string]any)
	}
	return
}

// 获取指定的key，结果返回map[string]string类型
func (c *Context) GetStringMapString(key string) (sms map[string]string) {
	if val, ok := c.Get(key); ok && val != nil {
		sms, _ = val.(map[string]string)
	}
	return
}

// 获取指定的key，结果返回map[string][]string类型
func (c *Context) GetStringMapStringSlice(key string) (smss map[string][]string) {
	if val, ok := c.Get(key); ok && val != nil {
		smss, _ = val.(map[string][]string)
	}
	return
}

/************************************/
/************ INPUT DATA ************/
/************************************/

// 返回URL的param值
//
//	router.GET("/user/:id", func(c *gin.Context) {
//	    // a GET request to /user/john
//	    id := c.Param("id") // id == "/john"
//	})
func (c *Context) Param(key string) string {
	return c.Params.ByName(key)
}

// 替换URL的param，添加到Context的Param中
//
// Example Route: "/user/:id"
// AddParam("id", 1)
// Result: "/user/1"
func (c *Context) AddParam(key, value string) {
	c.Params = append(c.Params, Param{Key: key, Value: value})
}

// 返回URL中对应key的值，不存在返回空字符串
//
// GET /path?id=1234&name=Manu&value=
//
//	c.Query("id") == "1234"
//	c.Query("name") == "Manu"
//	c.Query("value") == ""
//	c.Query("wtf") == ""
func (c *Context) Query(key string) (value string) {
	value, _ = c.GetQuery(key)
	return
}

// 返回URL中对应key的值，不存在返回设置的defaultValue
//
//	GET /?name=Manu&lastname=
//	c.DefaultQuery("name", "unknown") == "Manu"
//	c.DefaultQuery("id", "none") == "none"
//	c.DefaultQuery("lastname", "none") == ""
func (c *Context) DefaultQuery(key, defaultValue string) string {
	if value, ok := c.GetQuery(key); ok {
		return value
	}
	return defaultValue
}

// 返回URL中对应key的值，不存在返回空字符串，和Query()相比多一个bool位
//
//	GET /?name=Manu&lastname=
//	("Manu", true) == c.GetQuery("name")
//	("", false) == c.GetQuery("id")
//	("", true) == c.GetQuery("lastname")
func (c *Context) GetQuery(key string) (string, bool) {
	if values, ok := c.GetQueryArray(key); ok {
		return values[0], ok
	}
	return "", false
}

// 返回URL中对应key的[]string，其长度取决于key的参数数量
func (c *Context) QueryArray(key string) (values []string) {
	values, _ = c.GetQueryArray(key)
	return
}

// 初始化QueryCache
func (c *Context) initQueryCache() {
	if c.queryCache == nil {
		// c.Request不为空赋值为c.Request.URL.Query()
		if c.Request != nil {
			c.queryCache = c.Request.URL.Query()
		} else {
			c.queryCache = url.Values{}
		}
	}
}

// 返回URL中对应key的[]string，有一个存在返回true
func (c *Context) GetQueryArray(key string) (values []string, ok bool) {
	c.initQueryCache()
	values, ok = c.queryCache[key]
	return
}

// 返回URL中对应key的map[string]string
func (c *Context) QueryMap(key string) (dicts map[string]string) {
	dicts, _ = c.GetQueryMap(key)
	return
}

// 返回URL中对应key的map[string]string，有一个存在返回true
func (c *Context) GetQueryMap(key string) (map[string]string, bool) {
	c.initQueryCache()
	return c.get(c.queryCache, key)
}

// 从urlencoded form或multipart form获取指定的key，不存在返回空字符串
func (c *Context) PostForm(key string) (value string) {
	value, _ = c.GetPostForm(key)
	return
}

// 从urlencoded form或multipart form获取指定的key，不存在返回设置的defaultValue
func (c *Context) DefaultPostForm(key, defaultValue string) string {
	if value, ok := c.GetPostForm(key); ok {
		return value
	}
	return defaultValue
}

// 从urlencoded form或multipart form获取指定的key，不存在返回空字符串，和PostForm()相比多一个bool位
//
//	    email=mail@example.com  -->  ("mail@example.com", true) := GetPostForm("email") // set email to "mail@example.com"
//		   email=                  -->  ("", true) := GetPostForm("email") // set email to ""
//	                            -->  ("", false) := GetPostForm("email") // do nothing with email
func (c *Context) GetPostForm(key string) (string, bool) {
	if values, ok := c.GetPostFormArray(key); ok {
		return values[0], ok
	}
	return "", false
}

// 返回urlencoded form或multipart form中对应key的[]string，其长度取决于key的参数数量
func (c *Context) PostFormArray(key string) (values []string) {
	values, _ = c.GetPostFormArray(key)
	return
}

// 初始化FormCache
func (c *Context) initFormCache() {
	if c.formCache == nil {
		c.formCache = make(url.Values)
		req := c.Request
		// 使用MaxMultipartMemory进行ParseMultipartForm
		if err := req.ParseMultipartForm(c.engine.MaxMultipartMemory); err != nil {
			if !errors.Is(err, http.ErrNotMultipart) {
				debugPrint("error on parse multipart form array: %v", err)
			}
		}
		c.formCache = req.PostForm
	}
}

// 返回urlencoded form或multipart form中对应key的[]string，有一个存在返回true
func (c *Context) GetPostFormArray(key string) (values []string, ok bool) {
	c.initFormCache()
	values, ok = c.formCache[key]
	return
}

// 返回urlencoded form或multipart form中对应key的map[string]string
func (c *Context) PostFormMap(key string) (dicts map[string]string) {
	dicts, _ = c.GetPostFormMap(key)
	return
}

// 返回urlencoded form或multipart form中对应key的map[string]string，有一个存在返回true
func (c *Context) GetPostFormMap(key string) (map[string]string, bool) {
	c.initFormCache()
	return c.get(c.formCache, key)
}

// 私有方法，返回一个满足条件的map
func (c *Context) get(m map[string][]string, key string) (map[string]string, bool) {
	dicts := make(map[string]string)
	exist := false
	for k, v := range m {
		// 判断key的出现字符（[）之前有字符，并且k[0:i]和key是相等的
		if i := strings.IndexByte(k, '['); i >= 1 && k[0:i] == key {
			// 基于之前的判断，在k[i+1:]区间内还有字符（]）
			if j := strings.IndexByte(k[i+1:], ']'); j >= 1 {
				// 找到了满足的键值对
				exist = true
				// 将获取到的内容放到dicts[k[i+1:][:j]]位置
				dicts[k[i+1:][:j]] = v[0]
			}
		}
	}
	return dicts, exist
}

// 从MultipartForm中获取第一个匹配到name的file
func (c *Context) FormFile(name string) (*multipart.FileHeader, error) {
	// 获取file之前，需要对MultipartForm进行固定内存大小的解析，超过固定的内存大小，会将文件存储在磁盘上
	if c.Request.MultipartForm == nil {
		if err := c.Request.ParseMultipartForm(c.engine.MaxMultipartMemory); err != nil {
			return nil, err
		}
	}
	// 获取key为name的file
	f, fh, err := c.Request.FormFile(name)
	if err != nil {
		return nil, err
	}
	f.Close()
	return fh, err
}

// 解析MultipartForm，包括文件上传
func (c *Context) MultipartForm() (*multipart.Form, error) {
	// 解析成功的file会保存在c.Request.MultipartForm之中
	err := c.Request.ParseMultipartForm(c.engine.MaxMultipartMemory)
	return c.Request.MultipartForm, err
}

// 将上传的form file保存在指定的磁盘路径
func (c *Context) SaveUploadedFile(file *multipart.FileHeader, dst string) error {
	src, err := file.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	// 创建file文件夹，设置0750权限
	if err = os.MkdirAll(filepath.Dir(dst), 0750); err != nil {
		return err
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	// stream copy（src -> out）
	_, err = io.Copy(out, src)
	return err
}

// 通过Content-Type选择对应的binding engine（多态）
// 若input无效，则status重写为400，Content-Type设置为text/plain，阻止后续请求
//
//	"application/json" --> JSON binding
//	"application/xml"  --> XML binding
func (c *Context) Bind(obj any) error {
	// 通过Method和Content-Type获取默认的binding engine
	b := binding.Default(c.Request.Method, c.ContentType())
	return c.MustBindWith(obj, b)
}

// binding JSON类型
func (c *Context) BindJSON(obj any) error {
	return c.MustBindWith(obj, binding.JSON)
}

// binding XML类型
func (c *Context) BindXML(obj any) error {
	return c.MustBindWith(obj, binding.XML)
}

// binding Query类型
func (c *Context) BindQuery(obj any) error {
	return c.MustBindWith(obj, binding.Query)
}

// binding YAML类型
func (c *Context) BindYAML(obj any) error {
	return c.MustBindWith(obj, binding.YAML)
}

// binding TOML类型
func (c *Context) BindTOML(obj any) error {
	return c.MustBindWith(obj, binding.TOML)
}

// binding Header类型
func (c *Context) BindHeader(obj any) error {
	return c.MustBindWith(obj, binding.Header)
}

// binding Uri类型
func (c *Context) BindUri(obj any) error {
	if err := c.ShouldBindUri(obj); err != nil {
		// 出现错误重写status code为400
		c.AbortWithError(http.StatusBadRequest, err).SetType(ErrorTypeBind)
		return err
	}
	return nil
}

// 通过指定的binding engine，出现错误重写status code为400，并且调用AbortWithError阻止后续请求
func (c *Context) MustBindWith(obj any, b binding.Binding) error {
	if err := c.ShouldBindWith(obj, b); err != nil {
		c.AbortWithError(http.StatusBadRequest, err).SetType(ErrorTypeBind) //nolint: errcheck
		return err
	}
	return nil
}

// 通过Content-Type选择对应的binding engine（多态）
// 与Bind不同的是，若input无效，不会阻止后续操作、改变status code以及返回错误
//
//	"application/json" --> JSON binding
//	"application/xml"  --> XML binding
func (c *Context) ShouldBind(obj any) error {
	// 通过Method和Content-Type获取默认的binding engine
	b := binding.Default(c.Request.Method, c.ContentType())
	return c.ShouldBindWith(obj, b)
}

// should binding JSON类型
func (c *Context) ShouldBindJSON(obj any) error {
	return c.ShouldBindWith(obj, binding.JSON)
}

// should binding XML类型
func (c *Context) ShouldBindXML(obj any) error {
	return c.ShouldBindWith(obj, binding.XML)
}

// should binding Query类型
func (c *Context) ShouldBindQuery(obj any) error {
	return c.ShouldBindWith(obj, binding.Query)
}

// should binding YAML类型
func (c *Context) ShouldBindYAML(obj any) error {
	return c.ShouldBindWith(obj, binding.YAML)
}

// should binding TOML类型
func (c *Context) ShouldBindTOML(obj any) error {
	return c.ShouldBindWith(obj, binding.TOML)
}

// should binding Header类型
func (c *Context) ShouldBindHeader(obj any) error {
	return c.ShouldBindWith(obj, binding.Header)
}

// should binding Uri类型
func (c *Context) ShouldBindUri(obj any) error {
	m := make(map[string][]string)
	// 获取Params值，进行参数绑定
	for _, v := range c.Params {
		m[v.Key] = []string{v.Value}
	}
	return binding.Uri.BindUri(m, obj)
}

// 通过传入的obj进行参数绑定，obj需要是指针类型，should非强制性，不会报错和阻止请求
func (c *Context) ShouldBindWith(obj any, b binding.Binding) error {
	return b.Bind(c.Request, obj)
}

// ShouldBindBodyWith和ShouldBindWith作用类似，但是ShouldBindBodyWith会保存request body到context，方便下次使用
// 如果没有多次使用的需求的话，使用ShouldBindWith就可以，也可以提升一部分性能
func (c *Context) ShouldBindBodyWith(obj any, bb binding.BindingBody) (err error) {
	var body []byte
	// 尝试获取BodyBytesKey的值
	if cb, ok := c.Get(BodyBytesKey); ok {
		if cbb, ok := cb.([]byte); ok {
			body = cbb
		}
	}
	// 没有获取到BodyBytesKey的值
	if body == nil {
		// 从c.Request.Body读取body
		body, err = io.ReadAll(c.Request.Body)
		if err != nil {
			return err
		}
		// 将body的值写入BodyBytesKey的key/value中
		c.Set(BodyBytesKey, body)
	}
	// 使用[]body进行值绑定
	return bb.BindBody(body, obj)
}

// ClientIP方法尽可能获取到真实的访问IP，通过调用c.RemoteIP()来检查远程IP是否是受信任的代理。
// 若是受信任的代理，将尝试解析Engine.RemoteIPHeaders中定义的标头（默认为[X-Forwarded-For, X-Real-Ip]）
// 若不是受信任的代理，将返回来自Request.RemoteAddr的远程IP
func (c *Context) ClientIP() string {
	// 检查是否运行在信任的平台上，出现错误继续先后执行
	if c.engine.TrustedPlatform != "" {
		// 可以设置自己可信任或者预定义的platform
		if addr := c.requestHeader(c.engine.TrustedPlatform); addr != "" {
			return addr
		}
	}

	// AppEngine已经被遗弃，现在通过c.engine.TrustedPlatform的key进行设置
	if c.engine.AppEngine {
		log.Println(`The AppEngine flag is going to be deprecated. Please check issues #2723 and #2739 and use 'TrustedPlatform: gin.PlatformGoogleAppEngine' instead.`)
		if addr := c.requestHeader("X-Appengine-Remote-Addr"); addr != "" {
			return addr
		}
	}

	// 校验remoteIP是否可信任，执行此验证，它将查看IP是否包含在至少一个CIDR块中
	remoteIP := net.ParseIP(c.RemoteIP())
	if remoteIP == nil {
		return ""
	}
	// 校验是否为可信任的proxy
	trusted := c.engine.isTrustedProxy(remoteIP)

	// 如果不是信任的ip，直接返回
	if trusted && c.engine.ForwardedByClientIP && c.engine.RemoteIPHeaders != nil {
		for _, headerName := range c.engine.RemoteIPHeaders {
			// 校验header
			ip, valid := c.engine.validateHeader(c.requestHeader(headerName))
			if valid {
				return ip
			}
		}
	}
	return remoteIP.String()
}

// 从c.Request.RemoteAddr获取远程ip地址，不包括端口号
func (c *Context) RemoteIP() string {
	ip, _, err := net.SplitHostPort(strings.TrimSpace(c.Request.RemoteAddr))
	if err != nil {
		return ""
	}
	return ip
}

// 返回header的Content-Type值
func (c *Context) ContentType() string {
	return filterFlags(c.requestHeader("Content-Type"))
}

// 返回是否为websocket，握手由客户端发起。
func (c *Context) IsWebsocket() bool {
	if strings.Contains(strings.ToLower(c.requestHeader("Connection")), "upgrade") &&
		strings.EqualFold(c.requestHeader("Upgrade"), "websocket") {
		return true
	}
	return false
}

// 获取指定key的header值
func (c *Context) requestHeader(key string) string {
	return c.Request.Header.Get(key)
}

/************************************/
/******** RESPONSE RENDERING ********/
/************************************/

// 根据不同的status，判断是否需要body，从http.bodyAllowedForStatus copy的私有方法
func bodyAllowedForStatus(status int) bool {
	switch {
	case status >= 100 && status <= 199:
		return false
	case status == http.StatusNoContent:
		return false
	case status == http.StatusNotModified:
		return false
	}
	return true
}

// 设置http status code
func (c *Context) Status(code int) {
	c.Writer.WriteHeader(code)
}

// 设置response header
func (c *Context) Header(key, value string) {
	// 如果value为空，删除header的key
	if value == "" {
		c.Writer.Header().Del(key)
		return
	}
	// 设置header的key和value
	c.Writer.Header().Set(key, value)
}

// 返回header中key对应的值
func (c *Context) GetHeader(key string) string {
	return c.requestHeader(key)
}

// 返回body中的stream data
func (c *Context) GetRawData() ([]byte, error) {
	return io.ReadAll(c.Request.Body)
}

// 防止跨站请求伪造（CSRF）
func (c *Context) SetSameSite(samesite http.SameSite) {
	c.sameSite = samesite
}

// 将Set-Cookie添加到ResponseWriter的header中，提供的name必须是可用的，否则会被删除
func (c *Context) SetCookie(name, value string, maxAge int, path, domain string, secure, httpOnly bool) {
	if path == "" {
		path = "/"
	}
	// 添加cookie
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    url.QueryEscape(value),
		MaxAge:   maxAge,
		Path:     path,
		Domain:   domain,
		SameSite: c.sameSite,
		Secure:   secure,
		HttpOnly: httpOnly,
	})
}

// 返回名为name的cookie，没找到会返回ErrNoCookie错误，返回的cookie是未转义的
// 如果匹配到多个cookie，则只会返回一个
func (c *Context) Cookie(name string) (string, error) {
	// 通过name获取cookie
	cookie, err := c.Request.Cookie(name)
	if err != nil {
		return "", err
	}
	// 返回未转义的cookie
	val, _ := url.QueryUnescape(cookie.Value)
	return val, nil
}

// 写入response headers同时render数据
func (c *Context) Render(code int, r render.Render) {
	// 写入status code
	c.Status(code)

	// 不需要写入body的status code
	if !bodyAllowedForStatus(code) {
		// 通过不同的Content-Type，写入header
		r.WriteContentType(c.Writer)
		c.Writer.WriteHeaderNow()
		return
	}

	// 通过不同的Render实现，写入对应的数据，例如：Content-Type为JSON，调用JSON的Render回显数据
	if err := r.Render(c.Writer); err != nil {
		// 将err写入Error
		_ = c.Error(err)
		// 停止请求链路
		c.Abort()
	}
}

// HTML renders the HTTP template specified by its file name.
// It also updates the HTTP code and sets the Content-Type as "text/html".
// See http://golang.org/doc/articles/wiki/
// 通过指定的file name进行HTTP template Render，设置status code，同时设置Content-Type为"text/html"
func (c *Context) HTML(code int, name string, obj any) {
	// 获取HTML Render实例
	instance := c.engine.HTMLRender.Instance(name, obj)
	// 使用HTML Render
	c.Render(code, instance)
}

// 生成IndentedJSON在response body，设置Content-Type为"application/json"
// 使用IndentedJSON()会消耗更多的CPU和带宽，最好使用Context.JSON()来代替
func (c *Context) IndentedJSON(code int, obj any) {
	c.Render(code, render.IndentedJSON{Data: obj})
}

// 生成SecureJSON写入response body，设置Content-Type为"application/json"
func (c *Context) SecureJSON(code int, obj any) {
	c.Render(code, render.SecureJSON{Prefix: c.engine.secureJSONPrefix, Data: obj})
}

// 生成JSONP写入response body，设置Content-Type为"application/javascript"
func (c *Context) JSONP(code int, obj any) {
	callback := c.DefaultQuery("callback", "")
	if callback == "" {
		c.Render(code, render.JSON{Data: obj})
		return
	}
	c.Render(code, render.JsonpJSON{Callback: callback, Data: obj})
}

// 生成JSON写入response body，设置Content-Type为"application/json"
func (c *Context) JSON(code int, obj any) {
	c.Render(code, render.JSON{Data: obj})
}

// 生成AsciiJSON写入response body，设置Content-Type为"application/json"
func (c *Context) AsciiJSON(code int, obj any) {
	c.Render(code, render.AsciiJSON{Data: obj})
}

// 生成PureJSON写入response body，设置Content-Type为"application/json"
func (c *Context) PureJSON(code int, obj any) {
	c.Render(code, render.PureJSON{Data: obj})
}

// 生成XML写入response body，设置Content-Type为"application/xml"
func (c *Context) XML(code int, obj any) {
	c.Render(code, render.XML{Data: obj})
}

// 生成YAML写入response body，设置Content-Type为"application/x-yaml"
func (c *Context) YAML(code int, obj any) {
	c.Render(code, render.YAML{Data: obj})
}

// 生成TOML写入response body，设置Content-Type为"application/toml"
func (c *Context) TOML(code int, obj any) {
	c.Render(code, render.TOML{Data: obj})
}

// 生成ProtoBuf写入response body，设置Content-Type为"application/x-protobuf"
func (c *Context) ProtoBuf(code int, obj any) {
	c.Render(code, render.ProtoBuf{Data: obj})
}

// 生成String写入response body，设置Content-Type为"text/plain"
func (c *Context) String(code int, format string, values ...any) {
	c.Render(code, render.String{Format: format, Data: values})
}

// 重定向到指定的location地址
func (c *Context) Redirect(code int, location string) {
	c.Render(-1, render.Redirect{
		Code:     code,
		Location: location,
		Request:  c.Request,
	})
}

// 将[]byte stream数据写入response body，并修改status code
func (c *Context) Data(code int, contentType string, data []byte) {
	c.Render(code, render.Data{
		ContentType: contentType,
		Data:        data,
	})
}

// 将指定的render写入body stream
func (c *Context) DataFromReader(code int, contentLength int64, contentType string, reader io.Reader, extraHeaders map[string]string) {
	c.Render(code, render.Reader{
		Headers:       extraHeaders,
		ContentType:   contentType,
		ContentLength: contentLength,
		Reader:        reader,
	})
}

// 将指定的file写入body stream
func (c *Context) File(filepath string) {
	http.ServeFile(c.Writer, c.Request, filepath)
}

// 将http.FileSystem的file以高效的方式写入body stream
func (c *Context) FileFromFS(filepath string, fs http.FileSystem) {
	defer func(old string) {
		c.Request.URL.Path = old
	}(c.Request.URL.Path)

	// 设置filepath
	c.Request.URL.Path = filepath

	http.FileServer(fs).ServeHTTP(c.Writer, c.Request)
}

var quoteEscaper = strings.NewReplacer("\\", "\\\\", `"`, "\\\"")

// 替换quoteEscaper规则中的string
func escapeQuotes(s string) string {
	return quoteEscaper.Replace(s)
}

// 将指定的file以高效的方式写入body stream，客户端通过attachment指定filename进行下载
func (c *Context) FileAttachment(filepath, filename string) {
	if isASCII(filename) {
		c.Writer.Header().Set("Content-Disposition", `attachment; filename="`+escapeQuotes(filename)+`"`)
	} else {
		c.Writer.Header().Set("Content-Disposition", `attachment; filename*=UTF-8''`+url.QueryEscape(filename))
	}
	http.ServeFile(c.Writer, c.Request, filepath)
}

// 将服务器发送事件写入body stream
func (c *Context) SSEvent(name string, message any) {
	c.Render(-1, sse.Event{
		Event: name,
		Data:  message,
	})
}

// 发出stream response并返回bool值，判断client是否断开流
func (c *Context) Stream(step func(w io.Writer) bool) bool {
	w := c.Writer
	clientGone := w.CloseNotify()
	for {
		select {
		case <-clientGone: //　判断client是否连接
			return true
		default:
			keepOpen := step(w)
			w.Flush()
			if !keepOpen {
				return false
			}
		}
	}
}

/************************************/
/******** CONTENT NEGOTIATION *******/
/************************************/

// 包含Negotiate数据
type Negotiate struct {
	Offered  []string
	HTMLName string
	HTMLData any
	JSONData any
	XMLData  any
	YAMLData any
	Data     any
	TOMLData any
}

// 根据范围内的Content-Type类型调用对应的Render
func (c *Context) Negotiate(code int, config Negotiate) {
	switch c.NegotiateFormat(config.Offered...) {
	case binding.MIMEJSON:
		data := chooseData(config.JSONData, config.Data)
		c.JSON(code, data)

	case binding.MIMEHTML:
		data := chooseData(config.HTMLData, config.Data)
		c.HTML(code, config.HTMLName, data)

	case binding.MIMEXML:
		data := chooseData(config.XMLData, config.Data)
		c.XML(code, data)

	case binding.MIMEYAML:
		data := chooseData(config.YAMLData, config.Data)
		c.YAML(code, data)

	case binding.MIMETOML:
		data := chooseData(config.TOMLData, config.Data)
		c.TOML(code, data)

	default: // offered类型不匹配返回StatusNotAcceptable错误
		c.AbortWithError(http.StatusNotAcceptable, errors.New("the accepted formats are not offered by the server"))
	}
}

// NegotiateFormat returns an acceptable Accept format.
func (c *Context) NegotiateFormat(offered ...string) string {
	assert1(len(offered) > 0, "you must provide at least one offer")

	if c.Accepted == nil {
		c.Accepted = parseAccept(c.requestHeader("Accept"))
	}
	if len(c.Accepted) == 0 {
		return offered[0]
	}
	for _, accepted := range c.Accepted {
		for _, offer := range offered {
			// According to RFC 2616 and RFC 2396, non-ASCII characters are not allowed in headers,
			// therefore we can just iterate over the string without casting it into []rune
			i := 0
			for ; i < len(accepted) && i < len(offer); i++ {
				if accepted[i] == '*' || offer[i] == '*' {
					return offer
				}
				if accepted[i] != offer[i] {
					break
				}
			}
			if i == len(accepted) {
				return offer
			}
		}
	}
	return ""
}

// 设置Accept header数据
func (c *Context) SetAccepted(formats ...string) {
	c.Accepted = formats
}

/************************************/
/***** GOLANG.ORG/X/NET/CONTEXT *****/
/************************************/

// hasRequestContext returns whether c.Request has Context and fallback.
func (c *Context) hasRequestContext() bool {
	hasFallback := c.engine != nil && c.engine.ContextWithFallback
	hasRequestContext := c.Request != nil && c.Request.Context() != nil
	return hasFallback && hasRequestContext
}

// Deadline returns that there is no deadline (ok==false) when c.Request has no Context.
func (c *Context) Deadline() (deadline time.Time, ok bool) {
	if !c.hasRequestContext() {
		return
	}
	return c.Request.Context().Deadline()
}

// Done returns nil (chan which will wait forever) when c.Request has no Context.
func (c *Context) Done() <-chan struct{} {
	if !c.hasRequestContext() {
		return nil
	}
	return c.Request.Context().Done()
}

// Err returns nil when c.Request has no Context.
func (c *Context) Err() error {
	if !c.hasRequestContext() {
		return nil
	}
	return c.Request.Context().Err()
}

// Value returns the value associated with this context for key, or nil
// if no value is associated with key. Successive calls to Value with
// the same key returns the same result.
func (c *Context) Value(key any) any {
	if key == 0 {
		return c.Request
	}
	if key == ContextKey {
		return c
	}
	if keyAsString, ok := key.(string); ok {
		if val, exists := c.Get(keyAsString); exists {
			return val
		}
	}
	if !c.hasRequestContext() {
		return nil
	}
	return c.Request.Context().Value(key)
}
