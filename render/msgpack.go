// Copyright 2017 Manu Martinez-Almeida. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

//go:build !nomsgpack

package render

import (
	"net/http"

	"github.com/ugorji/go/codec"
)

var (
	// 确保MsgPack实现了Render接口
	_ Render = MsgPack{}
)

// MsgPack 结构体
type MsgPack struct {
	Data any
}

// msgpack的ContentType
var msgpackContentType = []string{"application/msgpack; charset=utf-8"}

// 将msgpackContentType写入header的ContentType
func (r MsgPack) WriteContentType(w http.ResponseWriter) {
	writeContentType(w, msgpackContentType)
}

// Render MsgPack数据
func (r MsgPack) Render(w http.ResponseWriter) error {
	return WriteMsgPack(w, r.Data)
}

// 写入ContentType和MsgPack数据
func WriteMsgPack(w http.ResponseWriter, obj any) error {
	// 先将msgpackContentType写入header的ContentType
	writeContentType(w, msgpackContentType)
	var mh codec.MsgpackHandle
	// echo obj数据，Encode包含了w.Writer操作
	return codec.NewEncoder(w, &mh).Encode(obj)
}
