// Copyright 2014 Manu Martinez-Almeida. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package gin

import (
	"flag"
	"io"
	"os"

	"github.com/gin-gonic/gin/binding"
)

// 默认读取的GinMode
const EnvGinMode = "GIN_MODE"

const (
	// debug模式
	DebugMode = "debug"
	// release模式
	ReleaseMode = "release"
	// test模式
	TestMode = "test"
)

const (
	debugCode = iota
	releaseCode
	testCode
)

// gin中使用的默认的io.Writer
var DefaultWriter io.Writer = os.Stdout

// gin中使用的默认的Error io.Writer
var DefaultErrorWriter io.Writer = os.Stderr

var (
	// 默认mode
	ginMode  = debugCode
	modeName = DebugMode
)

func init() {
	mode := os.Getenv(EnvGinMode)
	SetMode(mode)
}

// 设置gin mode
func SetMode(value string) {
	if value == "" {
		if flag.Lookup("test.v") != nil {
			value = TestMode
		} else {
			value = DebugMode
		}
	}

	switch value {
	case DebugMode:
		ginMode = debugCode
	case ReleaseMode:
		ginMode = releaseCode
	case TestMode:
		ginMode = testCode
	default:
		panic("gin mode unknown: " + value + " (available mode: debug release test)")
	}

	modeName = value
}

// 关闭默认validator
func DisableBindValidation() {
	binding.Validator = nil
}

// 设置binding.EnableDecoderUseNumber = true，以调用JSON Decoder实例上的UseNumber方法
func EnableJsonDecoderUseNumber() {
	binding.EnableDecoderUseNumber = true
}

// 设置binding.EnableDecoderDisallowUnknownFields = true，以调用JSON Decoder实例上的DisallowUnknownFields方法
func EnableJsonDecoderDisallowUnknownFields() {
	binding.EnableDecoderDisallowUnknownFields = true
}

// 返回当前的gin mode
func Mode() string {
	return modeName
}
