// Copyright 2014 Manu Martinez-Almeida. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package render

import "net/http"

// 基础存储数据struct
type Data struct {
	// Content-Type类型
	ContentType string
	// []byte的数据
	Data []byte
}

// 通过不同的Content-Type，echo对应的数据
func (r Data) Render(w http.ResponseWriter) (err error) {
	r.WriteContentType(w)
	// 写入数据，echo客户端
	_, err = w.Write(r.Data)
	return
}

// 通过Content-Type写入header
func (r Data) WriteContentType(w http.ResponseWriter) {
	writeContentType(w, []string{r.ContentType})
}
