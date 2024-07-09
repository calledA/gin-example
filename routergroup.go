// Copyright 2014 Manu Martinez-Almeida. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package gin

import (
	"net/http"
	"path"
	"regexp"
	"strings"
)

var (
	// 匹配http method，eg：POST
	regEnLetter = regexp.MustCompile("^[A-Z]+$")

	// RouterGroup的所有http method
	anyMethods = []string{
		http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch,
		http.MethodHead, http.MethodOptions, http.MethodDelete, http.MethodConnect,
		http.MethodTrace,
	}
)

// 包含所有router处理方法的接口（单个路由或者路由组）
type IRouter interface {
	IRoutes
	Group(string, ...HandlerFunc) *RouterGroup
}

// 包含所有router处理方法的接口
type IRoutes interface {
	Use(...HandlerFunc) IRoutes

	Handle(string, string, ...HandlerFunc) IRoutes
	Any(string, ...HandlerFunc) IRoutes
	GET(string, ...HandlerFunc) IRoutes
	POST(string, ...HandlerFunc) IRoutes
	DELETE(string, ...HandlerFunc) IRoutes
	PATCH(string, ...HandlerFunc) IRoutes
	PUT(string, ...HandlerFunc) IRoutes
	OPTIONS(string, ...HandlerFunc) IRoutes
	HEAD(string, ...HandlerFunc) IRoutes
	Match([]string, string, ...HandlerFunc) IRoutes

	StaticFile(string, string) IRoutes
	StaticFileFS(string, string, http.FileSystem) IRoutes
	Static(string, string) IRoutes
	StaticFS(string, http.FileSystem) IRoutes
}

// 用于路由配置（internally），与路由路径和HandlerFunc数组相关联
type RouterGroup struct {
	Handlers HandlersChain
	basePath string
	engine   *Engine
	root     bool
}

// 接口实现校验
var _ IRouter = (*RouterGroup)(nil)

// 添加一个middleware到RouterGroup
func (group *RouterGroup) Use(middleware ...HandlerFunc) IRoutes {
	group.Handlers = append(group.Handlers, middleware...)
	return group.returnObj()
}

// 创建一个新的RouterGroup，他们需要有相同的路由前缀和middleware
func (group *RouterGroup) Group(relativePath string, handlers ...HandlerFunc) *RouterGroup {
	return &RouterGroup{
		Handlers: group.combineHandlers(handlers),
		basePath: group.calculateAbsolutePath(relativePath),
		engine:   group.engine,
	}
}

// 返回RouterGroup的base path
// eg：if v := router.Group("/rest/n/v1/api"), v.BasePath() is "/rest/n/v1/api".
func (group *RouterGroup) BasePath() string {
	return group.basePath
}

// RouterGroup的处理函数
func (group *RouterGroup) handle(httpMethod, relativePath string, handlers HandlersChain) IRoutes {
	// 计算绝对路径
	absolutePath := group.calculateAbsolutePath(relativePath)
	// 将原有的handlers和传入的handlers进行结合
	handlers = group.combineHandlers(handlers)
	// 将http method、绝对路由路径、handlers添加到engine中
	group.engine.addRoute(httpMethod, absolutePath, handlers)
	return group.returnObj()
}

// 通过httpMethod和relativePath注册一个新的request handle
// 最后的handler必须是真实的handler，其他的可以是不同路由之间可以共享的middleware
func (group *RouterGroup) Handle(httpMethod, relativePath string, handlers ...HandlerFunc) IRoutes {
	// 查看httpMethod是否满足http method的要求
	if matched := regEnLetter.MatchString(httpMethod); !matched {
		panic("http method " + httpMethod + " is not valid")
	}

	return group.handle(httpMethod, relativePath, handlers)
}

// Handle的快捷注册函数（POST）
func (group *RouterGroup) POST(relativePath string, handlers ...HandlerFunc) IRoutes {
	return group.handle(http.MethodPost, relativePath, handlers)
}

// Handle的快捷注册函数（GET）
func (group *RouterGroup) GET(relativePath string, handlers ...HandlerFunc) IRoutes {
	return group.handle(http.MethodGet, relativePath, handlers)
}

// DELETE is a shortcut for router.Handle("DELETE", path, handlers).
// Handle的快捷注册函数（GET）
func (group *RouterGroup) DELETE(relativePath string, handlers ...HandlerFunc) IRoutes {
	return group.handle(http.MethodDelete, relativePath, handlers)
}

// PATCH is a shortcut for router.Handle("PATCH", path, handlers).
// Handle的快捷注册函数（GET）
func (group *RouterGroup) PATCH(relativePath string, handlers ...HandlerFunc) IRoutes {
	return group.handle(http.MethodPatch, relativePath, handlers)
}

// Handle的快捷注册函数（PUT）
func (group *RouterGroup) PUT(relativePath string, handlers ...HandlerFunc) IRoutes {
	return group.handle(http.MethodPut, relativePath, handlers)
}

// Handle的快捷注册函数（OPTIONS）
func (group *RouterGroup) OPTIONS(relativePath string, handlers ...HandlerFunc) IRoutes {
	return group.handle(http.MethodOptions, relativePath, handlers)
}

// Handle的快捷注册函数（HEAD）
func (group *RouterGroup) HEAD(relativePath string, handlers ...HandlerFunc) IRoutes {
	return group.handle(http.MethodHead, relativePath, handlers)
}

// Handle的快捷注册函数（匹配所有的http method）
// GET, POST, PUT, PATCH, HEAD, OPTIONS, DELETE, CONNECT, TRACE
func (group *RouterGroup) Any(relativePath string, handlers ...HandlerFunc) IRoutes {
	for _, method := range anyMethods {
		group.handle(method, relativePath, handlers)
	}

	return group.returnObj()
}

// Handle的快捷注册函数（匹配提供的http method）
func (group *RouterGroup) Match(methods []string, relativePath string, handlers ...HandlerFunc) IRoutes {
	for _, method := range methods {
		group.handle(method, relativePath, handlers)
	}

	return group.returnObj()
}

// 注册一个从本地文件系统下载单个文件的route
// eg：router.StaticFile("favicon.ico", "./resources/favicon.ico")
func (group *RouterGroup) StaticFile(relativePath, filepath string) IRoutes {
	return group.staticFileHandler(relativePath, func(c *Context) {
		c.File(filepath)
	})
}

// StaticFileFS与StaticFile作用类似，但是StaticFileFS可以指定http.FileSystem，默认为gin.Dir
// eg：router.StaticFileFS("favicon.ico", "./resources/favicon.ico", Dir{".", false})
func (group *RouterGroup) StaticFileFS(relativePath, filepath string, fs http.FileSystem) IRoutes {
	return group.staticFileHandler(relativePath, func(c *Context) {
		c.FileFromFS(filepath, fs)
	})
}

// 注册与校验file handler
func (group *RouterGroup) staticFileHandler(relativePath string, handler HandlerFunc) IRoutes {
	if strings.Contains(relativePath, ":") || strings.Contains(relativePath, "*") {
		panic("URL parameters can not be used when serving a static file")
	}
	// 注册路由路径到RouterGroup
	group.GET(relativePath, handler)
	group.HEAD(relativePath, handler)
	return group.returnObj()
}

// 从文件系统root提供file下载路径，使用http.FileSystem提供文件系统服务
func (group *RouterGroup) Static(relativePath, root string) IRoutes {
	return group.StaticFS(relativePath, Dir(root, false))
}

// StaticFS与Static相似，但是http.FileSystem可以被替换，默认使用gin.Dir
func (group *RouterGroup) StaticFS(relativePath string, fs http.FileSystem) IRoutes {
	if strings.Contains(relativePath, ":") || strings.Contains(relativePath, "*") {
		panic("URL parameters can not be used when serving a static folder")
	}
	// 创建一个static的handler
	handler := group.createStaticHandler(relativePath, fs)
	// path拼接
	urlPattern := path.Join(relativePath, "/*filepath")

	// 注册路由路径到RouterGroup
	group.GET(urlPattern, handler)
	group.HEAD(urlPattern, handler)
	return group.returnObj()
}

func (group *RouterGroup) createStaticHandler(relativePath string, fs http.FileSystem) HandlerFunc {
	// 计算绝对路径
	absolutePath := group.calculateAbsolutePath(relativePath)
	// 创建http的路由handler
	fileServer := http.StripPrefix(absolutePath, http.FileServer(fs))

	// 返回HandlerFunc
	return func(c *Context) {
		if _, noListing := fs.(*onlyFilesFS); noListing {
			c.Writer.WriteHeader(http.StatusNotFound)
		}

		// 获取param参数中的filepath
		file := c.Param("filepath")
		// 使用fs打开file
		f, err := fs.Open(file)
		// 报错返回404
		if err != nil {
			c.Writer.WriteHeader(http.StatusNotFound)
			c.handlers = group.engine.noRoute
			c.index = -1
			return
		}
		f.Close()

		// 开启file的http server
		fileServer.ServeHTTP(c.Writer, c.Request)
	}
}

// 将RouterGroup的HandlersChain和handlers的HandlersChain进行copy到一起
func (group *RouterGroup) combineHandlers(handlers HandlersChain) HandlersChain {
	finalSize := len(group.Handlers) + len(handlers)
	assert1(finalSize < int(abortIndex), "too many handlers")
	mergedHandlers := make(HandlersChain, finalSize)
	// HandlersChain复制
	copy(mergedHandlers, group.Handlers)
	copy(mergedHandlers[len(group.Handlers):], handlers)
	return mergedHandlers
}

// 计算路由的绝对路径
func (group *RouterGroup) calculateAbsolutePath(relativePath string) string {
	return joinPaths(group.basePath, relativePath)
}

// 返回IRoutes接口
func (group *RouterGroup) returnObj() IRoutes {
	// 根路由返回engine，否则返回group
	if group.root {
		return group.engine
	}
	return group
}
