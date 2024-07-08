// Copyright 2014 Manu Martinez-Almeida. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package render

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin/internal/bytesconv"
)

// 原始数据结构体
type String struct {
	// 格式化规则
	Format string
	// 数据切片
	Data []any
}

// plain的ContentType
var plainContentType = []string{"text/plain; charset=utf-8"}

// Render String数据
func (r String) Render(w http.ResponseWriter) error {
	return WriteString(w, r.Format, r.Data)
}

// 将plainContentType写入header的ContentType
func (r String) WriteContentType(w http.ResponseWriter) {
	writeContentType(w, plainContentType)
}

// 根据format规则echo数据
func WriteString(w http.ResponseWriter, format string, data []any) (err error) {
	// 先将plainContentType写入header的ContentType
	writeContentType(w, plainContentType)
	// 按照format写入数据到http.ResponseWriter中
	if len(data) > 0 {
		_, err = fmt.Fprintf(w, format, data...)
		return
	}
	// 将format写入http.ResponseWriter中
	_, err = w.Write(bytesconv.StringToBytes(format))
	return
}
