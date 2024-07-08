// Copyright 2014 Manu Martinez-Almeida. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package render

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"

	"github.com/gin-gonic/gin/internal/bytesconv"
	"github.com/gin-gonic/gin/internal/json"
)

// JSON 结构体
type JSON struct {
	Data any
}

// IndentedJSON（格式化JSON）结构体
type IndentedJSON struct {
	Data any
}

// SecureJSON（防止JSON劫持攻击）结构体
type SecureJSON struct {
	// 在返回的Json前添加前缀防止JSON劫持攻击
	Prefix string
	Data   any
}

// JsonpJSON（跨域请求）结构体
type JsonpJSON struct {
	// 回调函数的名称
	Callback string
	Data     any
}

// AsciiJSON（JSON 数据序列化为 ASCII 编码的字符串）结构体
type AsciiJSON struct {
	Data any
}

// PureJSON（紧凑的 JSON）结构体
type PureJSON struct {
	Data any
}

var (
	jsonContentType      = []string{"application/json; charset=utf-8"}
	jsonpContentType     = []string{"application/javascript; charset=utf-8"}
	jsonASCIIContentType = []string{"application/json"}
)

// Render JSON数据
func (r JSON) Render(w http.ResponseWriter) error {
	return WriteJSON(w, r.Data)
}

// 将jsonContent-Type写入header的Content-Type
func (r JSON) WriteContentType(w http.ResponseWriter) {
	writeContentType(w, jsonContentType)
}

// 写入JSON数据
func WriteJSON(w http.ResponseWriter, obj any) error {
	// 先将jsonContentType写入header的Content-Type
	writeContentType(w, jsonContentType)
	// 将obj进行Marshal转义
	jsonBytes, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	// 写入jsonBytes数据
	_, err = w.Write(jsonBytes)
	return err
}

// Render IndentedJSON数据
func (r IndentedJSON) Render(w http.ResponseWriter) error {
	// 先将jsonContentType写入header的Content-Type
	r.WriteContentType(w)
	// 将r.Data进行MarshalIndent转义
	jsonBytes, err := json.MarshalIndent(r.Data, "", "    ")
	if err != nil {
		return err
	}
	// 写入jsonBytes数据
	_, err = w.Write(jsonBytes)
	return err
}

// 将jsonContentType写入header的Content-Type
func (r IndentedJSON) WriteContentType(w http.ResponseWriter) {
	writeContentType(w, jsonContentType)
}

// Render SecureJSON数据以及prefix数据
func (r SecureJSON) Render(w http.ResponseWriter) error {
	// 先将jsonContentType写入header的Content-Type
	r.WriteContentType(w)
	// 将r.Data进行Marshal转义
	jsonBytes, err := json.Marshal(r.Data)
	if err != nil {
		return err
	}
	// 如果jsonBytes是Array数据
	if bytes.HasPrefix(jsonBytes, bytesconv.StringToBytes("[")) && bytes.HasSuffix(jsonBytes,
		bytesconv.StringToBytes("]")) {
		// 先将r.Prefix写入Writer
		if _, err = w.Write(bytesconv.StringToBytes(r.Prefix)); err != nil {
			return err
		}
	}
	// 写入jsonBytes数据
	_, err = w.Write(jsonBytes)
	return err
}

// 将jsonContentType写入header的ContentType
func (r SecureJSON) WriteContentType(w http.ResponseWriter) {
	writeContentType(w, jsonContentType)
}

// Render JsonpJSON数据以及对应的callback
func (r JsonpJSON) Render(w http.ResponseWriter) (err error) {
	// 先将jsonpContentType写入header的ContentType
	r.WriteContentType(w)
	// 将r.Data进行Marshal转义
	ret, err := json.Marshal(r.Data)
	if err != nil {
		return err
	}

	// 如果没有callback直接echo
	if r.Callback == "" {
		_, err = w.Write(ret)
		return err
	}

	// 通过处理返回JsonpJSON的数据，eg：handleResponse({"name":"Alice","age":30,"email":"alice@example.com"});
	callback := template.JSEscapeString(r.Callback)
	if _, err = w.Write(bytesconv.StringToBytes(callback)); err != nil {
		return err
	}

	if _, err = w.Write(bytesconv.StringToBytes("(")); err != nil {
		return err
	}

	if _, err = w.Write(ret); err != nil {
		return err
	}

	if _, err = w.Write(bytesconv.StringToBytes(");")); err != nil {
		return err
	}

	return nil
}

// 将jsonpContentType写入header的ContentType
func (r JsonpJSON) WriteContentType(w http.ResponseWriter) {
	writeContentType(w, jsonpContentType)
}

// Render AsciiJSON数据
func (r AsciiJSON) Render(w http.ResponseWriter) (err error) {
	// 先将jsonASCIIContentType写入header的ContentType
	r.WriteContentType(w)
	// 将r.Data进行Marshal转义
	ret, err := json.Marshal(r.Data)
	if err != nil {
		return err
	}

	var buffer bytes.Buffer
	for _, r := range bytesconv.BytesToString(ret) {
		cvt := string(r)
		// 对的非 ASCII 字符码值大于或等于 128 的字符进行Unicode 转义。
		if r >= 128 {
			// eg：'世'和'界'是非 ASCII 字符，被转换为\u4e16和\u754c。
			cvt = fmt.Sprintf("\\u%04x", int64(r))
		}
		buffer.WriteString(cvt)
	}

	// 写入buffer的数据
	_, err = w.Write(buffer.Bytes())
	return err
}

// 将jsonASCIIContentType写入header的ContentType
func (r AsciiJSON) WriteContentType(w http.ResponseWriter) {
	writeContentType(w, jsonASCIIContentType)
}

// Render PureJSON数据
func (r PureJSON) Render(w http.ResponseWriter) error {
	// 先将jsonContentType写入header的ContentType
	r.WriteContentType(w)
	// 创建新的json encoder
	encoder := json.NewEncoder(w)
	// 对JSON数据中的HTML字符不进行转义，eg：<, >, & 转义为 Unicode 转义序列\u003c, \u003e, \u0026
	encoder.SetEscapeHTML(false)
	// encoder.Encode进行w.Write返回数据
	return encoder.Encode(r.Data)
}

// 将jsonContentType写入header的ContentType
func (r PureJSON) WriteContentType(w http.ResponseWriter) {
	writeContentType(w, jsonContentType)
}
