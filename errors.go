// Copyright 2014 Manu Martinez-Almeida. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package gin

import (
	"fmt"
	"github.com/gin-gonic/gin/internal/json"
	"reflect"
	"strings"
)

// 使用uint64重新定义ErrorType
type ErrorType uint64

const (
	// Context Bind错误
	ErrorTypeBind ErrorType = 1 << 63
	// Context Render错误
	ErrorTypeRender ErrorType = 1 << 62
	// Private错误
	ErrorTypePrivate ErrorType = 1 << 0
	// Public错误
	ErrorTypePublic ErrorType = 1 << 1
	// Any other错误
	ErrorTypeAny ErrorType = 1<<64 - 1
	// Any other错误（和ErrorTypeAny解释一样，并且没用使用到，暂不清楚有什么用处）
	ErrorTypeNu = 2
)

// 自定义Error结构体
type Error struct {
	Err  error
	Type ErrorType
	Meta any
}

// Error列表
type errorMsgs []*Error

// 接口实现校验
var _ error = (*Error)(nil)

// 设置Error的ErrorType
func (msg *Error) SetType(flags ErrorType) *Error {
	msg.Type = flags
	return msg
}

// 设置Error的Meat Data
func (msg *Error) SetMeta(data any) *Error {
	msg.Meta = data
	return msg
}

// 创建正确格式的JSON
func (msg *Error) JSON() any {
	jsonData := H{}
	if msg.Meta != nil {
		// 反射判断Meta Data的类型
		value := reflect.ValueOf(msg.Meta)
		switch value.Kind() {
		case reflect.Struct: // 是reflect.Struct则直接返回msg.Meta
			return msg.Meta
		case reflect.Map: // 是reflect.Map则循环遍历msg.Meta进行赋值到jsonData
			for _, key := range value.MapKeys() {
				jsonData[key.String()] = value.MapIndex(key).Interface()
			}
		default: // 默认直接进行赋值到jsonData["meta"]
			jsonData["meta"] = msg.Meta
		}
	}
	// 如果有msg.Error()，则设置到jsonData["error"]
	if _, ok := jsonData["error"]; !ok {
		jsonData["error"] = msg.Error()
	}
	return jsonData
}

// 实现了json.Marshaller接口，对JSON数据进行格式化
func (msg *Error) MarshalJSON() ([]byte, error) {
	return json.Marshal(msg.JSON())
}

// 实现了error接口
func (msg Error) Error() string {
	return msg.Err.Error()
}

// 判断ErrorType
func (msg *Error) IsType(flags ErrorType) bool {
	return (msg.Type & flags) > 0
}

// 返回包装的错误，可以通过errors.Is()、errors.As()和errors.Unwrap()执行更多操作
func (msg *Error) Unwrap() error {
	return msg.Err
}

// 通过ErrorType返回过滤后的只读的切片，列如ByType(gin.ErrorTypePublic)，返回的切片值包含type等于ErrorTypePublic的元素
func (a errorMsgs) ByType(typ ErrorType) errorMsgs {
	if len(a) == 0 {
		return nil
	}
	// any返回所有值
	if typ == ErrorTypeAny {
		return a
	}

	// 判断类型，形成新的切片进行返回
	var result errorMsgs
	for _, msg := range a {
		if msg.IsType(typ) {
			result = append(result, msg)
		}
	}
	return result
}

// 返回errorMsgs最后一位的Error元素，如果errorMsgs为空则返回nil
func (a errorMsgs) Last() *Error {
	if length := len(a); length > 0 {
		return a[length-1]
	}
	return nil
}

// 返回errorMsgs所有Error元素的string类型的切片
func (a errorMsgs) Errors() []string {
	if len(a) == 0 {
		return nil
	}
	errorStrings := make([]string, len(a))
	for i, err := range a {
		errorStrings[i] = err.Error()
	}
	return errorStrings
}

// 对errorMsgs的Error元素进行JSON处理
func (a errorMsgs) JSON() any {
	switch length := len(a); length {
	case 0:
		return nil
	case 1:
		return a.Last().JSON()
	default:
		jsonData := make([]any, length)
		for i, err := range a {
			jsonData[i] = err.JSON()
		}
		return jsonData
	}
}

// 实现了json.Marshaller接口
func (a errorMsgs) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.JSON())
}

// 将errorMsgs进行字符串处理
func (a errorMsgs) String() string {
	if len(a) == 0 {
		return ""
	}
	// 使用strings.Builder将errorMsgs中的Err和Meta进行格式化输出
	var buffer strings.Builder
	for i, msg := range a {
		fmt.Fprintf(&buffer, "Error #%02d: %s\n", i+1, msg.Err)
		if msg.Meta != nil {
			fmt.Fprintf(&buffer, "     Meta: %v\n", msg.Meta)
		}
	}
	// 返回buffer的字符串
	return buffer.String()
}
