// Copyright 2014 Manu Martinez-Almeida. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package gin

import (
	"bufio"
	"io"
	"net"
	"net/http"
)

const (
	// 没有返回值
	noWritten = -1
	// 默认返回code
	defaultStatus = http.StatusOK
)

// ResponseWriter接口，继承了http库中的ResponseWriter、Hijacker、Flusher、CloseNotifier
type ResponseWriter interface {
	http.ResponseWriter
	http.Hijacker
	http.Flusher
	http.CloseNotifier

	// 返回的status code
	Status() int

	// response body返回的字节数
	Size() int

	// 写入string到response body中
	WriteString(string) (int, error)

	// 写入成功后返回true
	Written() bool

	// 强制写入http header
	WriteHeaderNow()

	// 返回http.Pusher
	Pusher() http.Pusher
}

// 封装的responseWriter结构体
type responseWriter struct {
	// http的ResponseWriter
	http.ResponseWriter
	// 返回的字节数
	size int
	// 返回的status code
	status int
}

// 接口实现校验
var _ ResponseWriter = (*responseWriter)(nil)

// Unwrap返回http的ResponseWriter
func (w *responseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// 重置responseWriter字段
func (w *responseWriter) reset(writer http.ResponseWriter) {
	w.ResponseWriter = writer
	w.size = noWritten
	w.status = defaultStatus
}

// 写入http header，code发生改变会重写header中的status code
func (w *responseWriter) WriteHeader(code int) {
	// code与status不同时
	if code > 0 && w.status != code {
		// 确认body写入完成
		if w.Written() {
			// debug打印重写header输出
			debugPrint("[WARNING] Headers were already written. Wanted to override status code %d with %d", w.status, code)
			return
		}
		// 重写header中的status code
		w.status = code
	}
}

// 强制写入http header
func (w *responseWriter) WriteHeaderNow() {
	// TODO：只有Written未完成时需要强制重写
	if !w.Written() {
		w.size = 0
		w.ResponseWriter.WriteHeader(w.status)
	}
}

// 重写http.ResponseWriter
func (w *responseWriter) Write(data []byte) (n int, err error) {
	// 写入header
	w.WriteHeaderNow()
	// 写入[]byte数据，并记录写入数据量
	n, err = w.ResponseWriter.Write(data)
	w.size += n
	return
}

// 实现ResponseWriter WriteString函数接口
func (w *responseWriter) WriteString(s string) (n int, err error) {
	// 写入header
	w.WriteHeaderNow()
	// 写入string数据，并记录写入数据量
	n, err = io.WriteString(w.ResponseWriter, s)
	w.size += n
	return
}

// 实现ResponseWriter Status函数接口
func (w *responseWriter) Status() int {
	return w.status
}

// 实现ResponseWriter Size函数接口
func (w *responseWriter) Size() int {
	return w.size
}

// 实现ResponseWriter Written函数接口
func (w *responseWriter) Written() bool {
	return w.size != noWritten
}

// 重写http.Hijacker
func (w *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if w.size < 0 {
		w.size = 0
	}
	return w.ResponseWriter.(http.Hijacker).Hijack()
}

// 重写http.CloseNotifier
func (w *responseWriter) CloseNotify() <-chan bool {
	return w.ResponseWriter.(http.CloseNotifier).CloseNotify()
}

// 重写http.Flusher
func (w *responseWriter) Flush() {
	w.WriteHeaderNow()
	w.ResponseWriter.(http.Flusher).Flush()
}

// 返回http.Pusher
func (w *responseWriter) Pusher() (pusher http.Pusher) {
	if pusher, ok := w.ResponseWriter.(http.Pusher); ok {
		return pusher
	}
	return nil
}
