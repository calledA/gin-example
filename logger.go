// Copyright 2014 Manu Martinez-Almeida. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package gin

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/mattn/go-isatty"
)

type consoleColorModeValue int

const (
	autoColor consoleColorModeValue = iota
	disableColor
	forceColor
)

const (
	green   = "\033[97;42m"
	white   = "\033[90;47m"
	yellow  = "\033[90;43m"
	red     = "\033[97;41m"
	blue    = "\033[97;44m"
	magenta = "\033[97;45m"
	cyan    = "\033[97;46m"
	reset   = "\033[0m"
)

var consoleColorMode = autoColor

// 定义Logger middleware
type LoggerConfig struct {
	// Logger的格式化输出，默认为gin.defaultLogFormatter
	Formatter LogFormatter

	// Logger的writter，默认为gin.DefaultWriter
	Output io.Writer

	// SkipPaths路径下的Logger将记录日志
	SkipPaths []string
}

// 格式化输出Logger的函数签名
type LogFormatter func(params LogFormatterParams) string

// 记录Logger的参数struct
type LogFormatterParams struct {
	// http request
	Request *http.Request

	// 服务器返回response的时间
	TimeStamp time.Time
	// status code
	StatusCode int
	// 服务器处理某个请求所花费的时间
	Latency time.Duration
	// ClientIP（Context的值）
	ClientIP string
	// http method
	Method string
	// client请求的path
	Path string
	// 当次请求中发生的错误信息
	ErrorMessage string
	// gin的output descriptor是否引用terminal
	isTerm bool
	//　response body的size大小
	BodySize int
	// Context设置的Keys
	Keys map[string]any
}

// 根据请求状态，设置terminal中的ANSI颜色
func (p *LogFormatterParams) StatusCodeColor() string {
	code := p.StatusCode

	switch {
	case code >= http.StatusOK && code < http.StatusMultipleChoices:
		return green
	case code >= http.StatusMultipleChoices && code < http.StatusBadRequest:
		return white
	case code >= http.StatusBadRequest && code < http.StatusInternalServerError:
		return yellow
	default:
		return red
	}
}

// 根据不同的request method，设置terminal中的ANSI颜色
func (p *LogFormatterParams) MethodColor() string {
	method := p.Method

	switch method {
	case http.MethodGet:
		return blue
	case http.MethodPost:
		return cyan
	case http.MethodPut:
		return yellow
	case http.MethodDelete:
		return red
	case http.MethodPatch:
		return green
	case http.MethodHead:
		return magenta
	case http.MethodOptions:
		return white
	default:
		return reset
	}
}

// 重设color
func (p *LogFormatterParams) ResetColor() string {
	return reset
}

// 是否可以将颜色输出到日志
func (p *LogFormatterParams) IsOutputColor() bool {
	return consoleColorMode == forceColor || (consoleColorMode == autoColor && p.isTerm)
}

// Logger middleware默认使用的日志格式函数
var defaultLogFormatter = func(param LogFormatterParams) string {
	var statusColor, methodColor, resetColor string
	if param.IsOutputColor() {
		statusColor = param.StatusCodeColor()
		methodColor = param.MethodColor()
		resetColor = param.ResetColor()
	}

	if param.Latency > time.Minute {
		param.Latency = param.Latency.Truncate(time.Second)
	}
	return fmt.Sprintf("[GIN] %v |%s %3d %s| %13v | %15s |%s %-7s %s %#v\n%s",
		param.TimeStamp.Format("2006/01/02 - 15:04:05"),
		statusColor, param.StatusCode, resetColor,
		param.Latency,
		param.ClientIP,
		methodColor, param.Method, resetColor,
		param.Path,
		param.ErrorMessage,
	)
}

// 禁止输出color到console
func DisableConsoleColor() {
	consoleColorMode = disableColor
}

// 强制输出color到console
func ForceConsoleColor() {
	consoleColorMode = forceColor
}

// ErrorLogger returns a HandlerFunc for any error type.
func ErrorLogger() HandlerFunc {
	return ErrorLoggerT(ErrorTypeAny)
}

// 通过指定的ErrorType返回一个HandlerFunc
func ErrorLoggerT(typ ErrorType) HandlerFunc {
	return func(c *Context) {
		c.Next()
		errors := c.Errors.ByType(typ)
		if len(errors) > 0 {
			c.JSON(-1, errors)
		}
	}
}

// 实例化一个Logger middleware，将日志写入gin.DefaultWriter，gin.DefaultWriter默认为os.Stdout
func Logger() HandlerFunc {
	return LoggerWithConfig(LoggerConfig{})
}

// 通过指定的LogFormatter实例化Logger middleware
func LoggerWithFormatter(f LogFormatter) HandlerFunc {
	return LoggerWithConfig(LoggerConfig{
		Formatter: f,
	})
}

// 通过指定的io.Writer实例化Logger middleware
func LoggerWithWriter(out io.Writer, notlogged ...string) HandlerFunc {
	return LoggerWithConfig(LoggerConfig{
		Output:    out,
		SkipPaths: notlogged,
	})
}

// 通过指定的LoggerConfig实例化Logger middleware
func LoggerWithConfig(conf LoggerConfig) HandlerFunc {
	// 设置formatter
	formatter := conf.Formatter
	if formatter == nil {
		formatter = defaultLogFormatter
	}

	//　设置output
	out := conf.Output
	if out == nil {
		out = DefaultWriter
	}

	// 跳过的path
	notlogged := conf.SkipPaths

	isTerm := true

	// 判断w的句柄是否为terminal
	if w, ok := out.(*os.File); !ok || os.Getenv("TERM") == "dumb" ||
		(!isatty.IsTerminal(w.Fd()) && !isatty.IsCygwinTerminal(w.Fd())) {
		isTerm = false
	}

	// skip map
	var skip map[string]struct{}

	if length := len(notlogged); length > 0 {
		skip = make(map[string]struct{}, length)

		for _, path := range notlogged {
			skip[path] = struct{}{}
		}
	}

	return func(c *Context) {
		// 开始时间
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		// 进行下一个处理请求
		c.Next()

		// path不在skip map中，则记录日志
		if _, ok := skip[path]; !ok {
			// LogFormatter参数
			param := LogFormatterParams{
				Request: c.Request,
				isTerm:  isTerm,
				Keys:    c.Keys,
			}

			// 记录数据
			param.TimeStamp = time.Now()

			param.Latency = param.TimeStamp.Sub(start)

			param.ClientIP = c.ClientIP()
			param.Method = c.Request.Method
			param.StatusCode = c.Writer.Status()
			param.ErrorMessage = c.Errors.ByType(ErrorTypePrivate).String()

			param.BodySize = c.Writer.Size()

			if raw != "" {
				path = path + "?" + raw
			}

			param.Path = path

			// 将formatter的数据写入到out stream中
			fmt.Fprint(out, formatter(param))
		}
	}
}
