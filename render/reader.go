// Copyright 2018 Gin Core Team. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package render

import (
	"io"
	"net/http"
	"strconv"
)

// Reader 结构体
type Reader struct {
	// ContentType类型
	ContentType string
	// IO reader的长度
	ContentLength int64
	// IO reader
	Reader io.Reader
	// 其他的headers
	Headers map[string]string
}

// Render echo数据以及对应的Headers
func (r Reader) Render(w http.ResponseWriter) (err error) {
	// 写入header的ContentType
	r.WriteContentType(w)
	// echo数据不为空
	if r.ContentLength >= 0 {
		// 设置默认的r.Headers
		if r.Headers == nil {
			r.Headers = map[string]string{}
		}
		// 写入header的ContentLength
		r.Headers["Content-Length"] = strconv.FormatInt(r.ContentLength, 10)
	}
	// 写入其他的header
	r.writeHeaders(w, r.Headers)
	// 将reader数据流写入到writer数据流
	_, err = io.Copy(w, r.Reader)
	return
}

// 将r.ContentType写入header的Content-Type
func (r Reader) WriteContentType(w http.ResponseWriter) {
	writeContentType(w, []string{r.ContentType})
}

// 写入header
func (r Reader) writeHeaders(w http.ResponseWriter, headers map[string]string) {
	header := w.Header()
	for k, v := range headers {
		// 需要写入的header为空则写入到header中
		if header.Get(k) == "" {
			header.Set(k, v)
		}
	}
}
