// Copyright 2013 Julien Schmidt. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/julienschmidt/httprouter/blob/master/LICENSE

package gin

import (
	"bytes"
	"github.com/gin-gonic/gin/internal/bytesconv"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	strColon = []byte(":")
	strStar  = []byte("*")
	strSlash = []byte("/")
)

// URL的参数（param），包含一对键值对
type Param struct {
	Key   string
	Value string
}

// 通过router返回的Param的有序切片，第一对URL的Param处于切片的第一位，因此通过index读取是安全的
type Params []Param

// 返回第一个匹配到的Param，同时返回true，如果没有匹配到值，则返回空字符串和false
func (ps Params) Get(name string) (string, bool) {
	for _, entry := range ps {
		if entry.Key == name {
			return entry.Value, true
		}
	}
	return "", false
}

// 返回第一个匹配到的Param，如果没有匹配到值，则返回空字符串
func (ps Params) ByName(name string) (va string) {
	va, _ = ps.Get(name)
	return
}

// 方法树
type methodTree struct {
	method string
	root   *node
}

// 方法树的切片
type methodTrees []methodTree

// 返回匹配的method的节点
func (trees methodTrees) get(method string) *node {
	for _, tree := range trees {
		// 匹配tree的节点
		if tree.method == method {
			return tree.root
		}
	}
	return nil
}

// 返回较小值
func min(a, b int) int {
	if a <= b {
		return a
	}
	return b
}

// 返回最大公共前缀
func longestCommonPrefix(a, b string) int {
	i := 0
	max := min(len(a), len(b))
	for i < max && a[i] == b[i] {
		i++
	}
	return i
}

// 添加一个child node，保持wildcardChild在最后一位
func (n *node) addChild(child *node) {
	if n.wildChild && len(n.children) > 0 {
		// 将wildcardChild拷贝出来放在最后一位
		wildcardChild := n.children[len(n.children)-1]
		n.children = append(n.children[:len(n.children)-1], child, wildcardChild)
	} else {
		// 直接添加到末尾
		n.children = append(n.children, child)
	}
}

// 对strColon和strStar计数
func countParams(path string) uint16 {
	var n uint16
	s := bytesconv.StringToBytes(path)
	n += uint16(bytes.Count(s, strColon))
	n += uint16(bytes.Count(s, strStar))
	return n
}

// 对strSlash计数
func countSections(path string) uint16 {
	s := bytesconv.StringToBytes(path)
	return uint16(bytes.Count(s, strSlash))
}

type nodeType uint8

const (
	static nodeType = iota
	root
	param
	catchAll
)

// TODO
type node struct {
	path    string
	indices string
	// 是否为通配符的节点
	wildChild bool
	// node类型
	nType nodeType
	// 当前node的优先级
	priority uint32
	children []*node // child nodes, at most 1 :param style node at the end of the array
	handlers HandlersChain
	fullPath string
}

// 增加所给child的优先级，在必要时重新排序
func (n *node) incrementChildPrio(pos int) int {
	// 找出children
	cs := n.children
	// 增加pos位置的children的优先级
	cs[pos].priority++
	prio := cs[pos].priority

	// 调整不同优先级的位置，将高优先级的替换到前面
	newPos := pos
	for ; newPos > 0 && cs[newPos-1].priority < prio; newPos-- {
		// 交换前后值
		cs[newPos-1], cs[newPos] = cs[newPos], cs[newPos-1]
	}

	// 更新indices的值，eg：pos为3,newPos为1，hello -> hlelo
	if newPos != pos {
		n.indices = n.indices[:newPos] + n.indices[pos:pos+1] + n.indices[newPos:pos] + n.indices[pos+1:]
	}

	return newPos
}

// 添加一个所给handler的node到path中，非线程安全
func (n *node) addRoute(path string, handlers HandlersChain) {
	fullPath := path
	// 每添加一个node，优先级++，node越多，优先级越高
	n.priority++

	// 空树情况
	if len(n.path) == 0 && len(n.children) == 0 {
		// 添加child node
		n.insertChild(path, fullPath, handlers)
		// 空树时，node设置为root
		n.nType = root
		return
	}

	parentFullPathIndex := 0

walk:
	for {
		// 找出最大公共前缀，公共前缀中不包含':'或'*'
		i := longestCommonPrefix(path, n.path)

		// 当最大公共前缀小于当前node的path
		if i < len(n.path) {
			// 创建新节点信息，新node的优先级要小于当前node
			child := node{
				path:      n.path[i:],
				wildChild: n.wildChild,
				nType:     static,
				indices:   n.indices,
				children:  n.children,
				handlers:  n.handlers,
				priority:  n.priority - 1,
				fullPath:  n.fullPath,
			}

			// 更新当前node信息
			n.children = []*node{&child}
			n.indices = bytesconv.BytesToString([]byte{n.path[i]})
			n.path = path[:i]
			n.handlers = nil
			n.wildChild = false
			n.fullPath = fullPath[:parentFullPathIndex+i]
		}

		// 使新节点成为当前节点的子节点
		if i < len(path) {
			// 最大公共前缀之后的path成为新的path
			path = path[i:]
			// 获取新path的第一个字符
			c := path[0]

			// 参数后有'/'
			if n.nType == param && c == '/' && len(n.children) == 1 {
				parentFullPathIndex += len(n.path)
				n = n.children[0]
				n.priority++
				continue walk
			}

			// 检查是否存在下一个path中的首字符，即选择字典树的子node，有可能找不到子node
			for i, max := 0, len(n.indices); i < max; i++ {
				// 查找当前node的indices中是否包含next path的首字符，eg：parent为hc，next path的首字符为h
				if c == n.indices[i] {
					parentFullPathIndex += len(n.path)
					i = n.incrementChildPrio(i)
					n = n.children[i]
					continue walk
				}
			}

			// 添加新node
			if c != ':' && c != '*' && n.nType != catchAll {
				// 将c添加到当前node的indices
				n.indices += bytesconv.BytesToString([]byte{c})
				child := &node{
					fullPath: fullPath,
				}
				// 当前node添加子node
				n.addChild(child)
				// 更新优先级
				n.incrementChildPrio(len(n.indices) - 1)
				n = child
			} else if n.wildChild {
				// 插入通配符node，插入之前检查是否和存在的通配符冲突
				n = n.children[len(n.children)-1]
				n.priority++

				// 检查通配符是否匹配，同一个路径下，只能有一个通配符
				if len(path) >= len(n.path) && n.path == path[:len(n.path)] &&
					// 当前node的nType不为catchAll
					n.nType != catchAll &&
					// 检查较长的通配符，e.g. :name and :names
					(len(n.path) >= len(path) || path[len(n.path)] == '/') {
					continue walk
				}

				// 通配符冲突
				pathSeg := path
				// 找到冲突的通配符位置，只取'/'分割的首个冲突位置
				if n.nType != catchAll {
					pathSeg = strings.SplitN(pathSeg, "/", 2)[0]
				}
				// 冲突的通配符path prefix
				prefix := fullPath[:strings.Index(fullPath, pathSeg)] + n.path
				panic("'" + pathSeg +
					"' in new path '" + fullPath +
					"' conflicts with existing wildcard '" + n.path +
					"' in existing prefix '" + prefix +
					"'")
			}

			n.insertChild(path, fullPath, handlers)
			return
		}

		// 给当前node添加handle
		if n.handlers != nil {
			panic("handlers are already registered for path '" + fullPath + "'")
		}
		n.handlers = handlers
		n.fullPath = fullPath
		return
	}
}

// 搜索通配符并检查是否包含非法字符，如果没有找到通配符，返回-1
func findWildcard(path string) (wildcard string, i int, valid bool) {
	// 开始查找非法字符
	for start, c := range []byte(path) {
		// 通配符是否包含':'(参数)或者'*'(匹配所有)
		if c != ':' && c != '*' {
			continue
		}

		// 非法字符处理
		valid = true
		for end, c := range []byte(path[start+1:]) {
			switch c {
			case '/': // 出现'/'，则进入下一个路由的匹配中
				return path[start : start+1+end], start, valid
			case ':', '*': // 最多只能包含一个':'或'*'，否则valid = false
				valid = false
			}
		}
		// 找到非法字符的返回
		return path[start:], start, valid
	}
	// 没有非法字符的返回
	return "", -1, false
}

// 添加tree node
func (n *node) insertChild(path string, fullPath string, handlers HandlersChain) {
	for {
		// 查找第一个通配符之前的前缀
		wildcard, i, valid := findWildcard(path)
		// 没有通配符
		if i < 0 {
			break
		}

		// 通配符中最多包含一个':'或'*'
		if !valid {
			panic("only one wildcard per path segment is allowed, has: '" +
				wildcard + "' in path '" + fullPath + "'")
		}

		// 检查通配符是否有名称，eg：':username'
		if len(wildcard) < 2 {
			panic("wildcards must be named with a non-empty name in path '" + fullPath + "'")
		}

		// wildcard为param的情况，eg：':name'
		if wildcard[0] == ':' {
			if i > 0 {
				// 当前node的通配符添加前缀
				n.path = path[:i]
				path = path[i:]
			}

			child := &node{
				nType:    param,
				path:     wildcard,
				fullPath: fullPath,
			}
			n.addChild(child)
			n.wildChild = true
			n = child
			n.priority++

			// 如果path不以通配符结尾，则将会有另一个以'/'开头的子路径
			if len(wildcard) < len(path) {
				path = path[len(wildcard):]

				child := &node{
					priority: 1,
					fullPath: fullPath,
				}
				n.addChild(child)
				n = child
				continue
			}

			// 将handler添加到新的叶子节点中
			n.handlers = handlers
			return
		}

		// 处理wildcard为catchAll的情况
		if i+len(wildcard) != len(path) { // '*'只能在path的末尾
			panic("catch-all routes are only allowed at the end of the path in path '" + fullPath + "'")
		}

		if len(n.path) > 0 && n.path[len(n.path)-1] == '/' { // TODO
			pathSeg := strings.SplitN(n.children[0].path, "/", 2)[0]
			panic("catch-all wildcard '" + path +
				"' in new path '" + fullPath +
				"' conflicts with existing path segment '" + pathSeg +
				"' in existing prefix '" + n.path + pathSeg +
				"'")
		}

		i--

		// 判断'*'通配符之前是不是'/'
		if path[i] != '/' {
			panic("no / before catch-all in path '" + fullPath + "'")
		}

		n.path = path[:i]

		// catchAll的第一个node的path为空，nType为catchAll
		child := &node{
			wildChild: true,
			nType:     catchAll,
			fullPath:  fullPath,
		}

		// 添加一个空catchAll node
		n.addChild(child)
		n.indices = string('/')
		n = child
		n.priority++

		// catchAll的node存储变量值，nType为catchAll
		child = &node{
			path:     path[i:],
			nType:    catchAll,
			handlers: handlers,
			priority: 1,
			fullPath: fullPath,
		}
		// 将存储变量信息的node添加到空catchAll node的children
		n.children = []*node{child}

		return
	}

	// 没有找到通配符，则只插入path和handle
	n.path = path
	n.handlers = handlers
	n.fullPath = fullPath
}

// 保存getValue的返回值
type nodeValue struct {
	handlers HandlersChain
	params   *Params
	tsr      bool
	fullPath string
}

// 跳过的node
type skippedNode struct {
	path        string
	node        *node
	paramsCount int16
}

// 返回path注册的handle。通配符的值保存到map中。
// 如果没有找到handle，如果path存在带有额外（不带）尾部'/'的handle，则tsr为trur
func (n *node) getValue(path string, params *Params, skippedNodes *[]skippedNode, unescape bool) (value nodeValue) {
	var globalParamsCount int16

walk: // 直到找到匹配的路径或没有更多节点可遍历为止
	for {
		// 处理path前缀
		prefix := n.path
		if len(path) > len(prefix) {
			if path[:len(prefix)] == prefix {
				path = path[len(prefix):]

				// 尝试匹配非通配符子node
				idxc := path[0]
				for i, c := range []byte(n.indices) {
					if c == idxc {
						// 有通配符子node，记录跳过的node
						if n.wildChild {
							index := len(*skippedNodes)
							*skippedNodes = (*skippedNodes)[:index+1]
							(*skippedNodes)[index] = skippedNode{
								path: prefix + path,
								node: &node{
									path:      n.path,
									wildChild: n.wildChild,
									nType:     n.nType,
									priority:  n.priority,
									children:  n.children,
									handlers:  n.handlers,
									fullPath:  n.fullPath,
								},
								paramsCount: globalParamsCount,
							}
						}

						// 继续遍历子node
						n = n.children[i]
						continue walk
					}
				}

				// 没有通配符子node时的处理
				if !n.wildChild {
					// 处理路径不为'/'的情况
					if path != "/" {
						for length := len(*skippedNodes); length > 0; length-- {
							skippedNode := (*skippedNodes)[length-1]
							*skippedNodes = (*skippedNodes)[:length-1]
							if strings.HasSuffix(skippedNode.path, path) {
								path = skippedNode.path
								n = skippedNode.node
								if value.params != nil {
									*value.params = (*value.params)[:skippedNode.paramsCount]
								}
								globalParamsCount = skippedNode.paramsCount
								continue walk
							}
						}
					}

					// 没有找到匹配的路径
					value.tsr = path == "/" && n.handlers != nil
					return
				}

				// 处理通配符子node,总是取当前node的children的最后一位
				n = n.children[len(n.children)-1]
				globalParamsCount++

				switch n.nType {
				case param:

					// 处理参数节点，找到param的尾部
					end := 0
					for end < len(path) && path[end] != '/' {
						end++
					}

					// 保存参数值
					if params != nil && cap(*params) > 0 {
						if value.params == nil {
							value.params = params
						}
						// 在预分配的容量范围内扩展切片
						i := len(*value.params)
						*value.params = (*value.params)[:i+1]
						val := path[:end]
						if unescape {
							if v, err := url.QueryUnescape(val); err == nil {
								val = v
							}
						}
						(*value.params)[i] = Param{
							Key:   n.path[1:],
							Value: val,
						}
					}

					// 继续处理剩余路径
					if end < len(path) {
						if len(n.children) > 0 {
							path = path[end:]
							n = n.children[0]
							continue walk
						}

						// 没有更多子节点
						value.tsr = len(path) == end+1
						return
					}

					// 处理找到的处理函数
					if value.handlers = n.handlers; value.handlers != nil {
						value.fullPath = n.fullPath
						return
					}
					if len(n.children) == 1 {
						// 检查是否存在带有'/'的路径
						n = n.children[0]
						value.tsr = (n.path == "/" && n.handlers != nil) || (n.path == "" && n.indices == "/")
					}
					return

				case catchAll:
					// 处理catchAll的节点
					if params != nil {
						if value.params == nil {
							value.params = params
						}
						// 在预分配的容量范围内扩展切片
						i := len(*value.params)
						*value.params = (*value.params)[:i+1]
						val := path
						if unescape {
							if v, err := url.QueryUnescape(path); err == nil {
								val = v
							}
						}
						(*value.params)[i] = Param{
							Key:   n.path[2:],
							Value: val,
						}
					}

					// 返回找到的处理函数
					value.handlers = n.handlers
					value.fullPath = n.fullPath
					return

				default:
					panic("invalid node type")
				}
			}
		}

		if path == prefix {
			// 如果当前path不等于'/'，且node没有handler，且最近匹配的node有children
			// 当前node需要回滚到最后一个有效的skippedNode
			if n.handlers == nil && path != "/" {
				for length := len(*skippedNodes); length > 0; length-- {
					skippedNode := (*skippedNodes)[length-1]
					*skippedNodes = (*skippedNodes)[:length-1]
					if strings.HasSuffix(skippedNode.path, path) {
						path = skippedNode.path
						n = skippedNode.node
						if value.params != nil {
							*value.params = (*value.params)[:skippedNode.paramsCount]
						}
						globalParamsCount = skippedNode.paramsCount
						continue walk
					}
				}
			}

			// 检查node是否有handler
			if value.handlers = n.handlers; value.handlers != nil {
				value.fullPath = n.fullPath
				return
			}

			// 如果path没有handler，但是当前node有子通配符，则path必须包含一个包含尾部'/'的handler
			if path == "/" && n.wildChild && n.nType != root {
				value.tsr = true
				return
			}

			if path == "/" && n.nType == static {
				value.tsr = true
				return
			}

			// 没有找到handler，检查是否存在此路径的handler + 尾部'/'
			for i, c := range []byte(n.indices) {
				if c == '/' {
					n = n.children[i]
					value.tsr = (len(n.path) == 1 && n.handlers != nil) ||
						(n.nType == catchAll && n.children[0].handlers != nil)
					return
				}
			}

			return
		}

		// 未找到任何内容时，如果该路径存在叶子节点,则重定向到相同的 URL，但要添加一个额外的尾部'/'
		value.tsr = path == "/" ||
			(len(prefix) == len(path)+1 && prefix[len(path)] == '/' &&
				path == prefix[:len(prefix)-1] && n.handlers != nil)

		// 回滚到最后一个有效的skippedNode
		if !value.tsr && path != "/" {
			for length := len(*skippedNodes); length > 0; length-- {
				skippedNode := (*skippedNodes)[length-1]
				*skippedNodes = (*skippedNodes)[:length-1]
				if strings.HasSuffix(skippedNode.path, path) {
					path = skippedNode.path
					n = skippedNode.node
					if value.params != nil {
						*value.params = (*value.params)[:skippedNode.paramsCount]
					}
					globalParamsCount = skippedNode.paramsCount
					continue walk
				}
			}
		}

		return
	}
}

// 对给定的path进行不区分大小写的查找，可以选择性的修复尾部'/'
// 返回大小写更正后的路径和一个布尔值
func (n *node) findCaseInsensitivePath(path string, fixTrailingSlash bool) ([]byte, bool) {
	const stackBufSize = 128

	// 创建stackBufSize大小的buf，如果path太长，则重新分配buf大小
	buf := make([]byte, 0, stackBufSize)
	if length := len(path) + 1; length > stackBufSize {
		buf = make([]byte, 0, length)
	}

	// 不区分大小写查找path
	ciPath := n.findCaseInsensitivePathRec(
		path,
		buf,
		[4]byte{},
		fixTrailingSlash,
	)

	return ciPath, ciPath != nil
}

// 将数组中byte左移n位
func shiftNRuneBytes(rb [4]byte, n int) [4]byte {
	switch n {
	case 0:
		return rb
	case 1:
		return [4]byte{rb[1], rb[2], rb[3], 0}
	case 2:
		return [4]byte{rb[2], rb[3]}
	case 3:
		return [4]byte{rb[3]}
	default:
		return [4]byte{}
	}
}

// 使用递归，不区分大小写查找path
func (n *node) findCaseInsensitivePathRec(path string, ciPath []byte, rb [4]byte, fixTrailingSlash bool) []byte {
	// 当前node path长度
	npLen := len(n.path)

walk: // 外层循环标签，继续遍历树
	for len(path) >= npLen && (npLen == 0 || strings.EqualFold(path[1:npLen], n.path[1:])) {
		// 检查当前路径前缀是否与节点路径匹配，且不区分大小写
		// 添加匹配的前缀到结果中
		oldPath := path
		path = path[npLen:]
		ciPath = append(ciPath, n.path...)

		if len(path) == 0 {
			// 如果路径完全匹配且当前节点有处理函数，返回结果路径
			if n.handlers != nil {
				return ciPath
			}

			// 尝试通过添加结尾斜杠修正路径
			if fixTrailingSlash {
				for i, c := range []byte(n.indices) {
					if c == '/' {
						n = n.children[i]
						if (len(n.path) == 1 && n.handlers != nil) ||
							(n.nType == catchAll && n.children[0].handlers != nil) {
							return append(ciPath, '/')
						}
						return nil
					}
				}
			}
			return nil
		}

		// 如果当前节点没有通配符（参数或捕获所有）子节点，则继续查找下一个子节点
		if !n.wildChild {
			// 跳过已经处理byte
			rb = shiftNRuneBytes(rb, npLen)

			if rb[0] != 0 {
				// 如果旧的rune未完成，继续处理
				idxc := rb[0]
				for i, c := range []byte(n.indices) {
					if c == idxc {
						// continue with child node
						n = n.children[i]
						npLen = len(n.path)
						continue walk
					}
				}
			} else {
				// 处理新的rune
				var rv rune

				// 查找rune起始位置
				var off int
				for max := min(npLen, 3); off < max; off++ {
					if i := npLen - off; utf8.RuneStart(oldPath[i]) {
						// 从缓存路径读取rune
						rv, _ = utf8.DecodeRuneInString(oldPath[i:])
						break
					}
				}

				// 计算当前rune的小写字节
				lo := unicode.ToLower(rv)
				utf8.EncodeRune(rb[:], lo)

				// 跳过已经处理的字节
				rb = shiftNRuneBytes(rb, off)

				idxc := rb[0]
				for i, c := range []byte(n.indices) {
					// 小写匹配
					if c == idxc {
						// 必须使用递归方法，因为大写和小写字节都可能作为索引存在
						if out := n.children[i].findCaseInsensitivePathRec(
							path, ciPath, rb, fixTrailingSlash,
						); out != nil {
							return out
						}
						break
					}
				}

				// 如果未找到匹配，尝试大写rune
				if up := unicode.ToUpper(rv); up != lo {
					utf8.EncodeRune(rb[:], up)
					rb = shiftNRuneBytes(rb, off)

					idxc := rb[0]
					for i, c := range []byte(n.indices) {
						// 大写匹配
						if c == idxc {
							// 继续处理子节点
							n = n.children[i]
							npLen = len(n.path)
							continue walk
						}
					}
				}
			}

			// 如果未找到任何匹配，可以推荐重定向到没有结尾斜杠的相同URL
			if fixTrailingSlash && path == "/" && n.handlers != nil {
				return ciPath
			}
			return nil
		}

		// 处理通配符子节点
		n = n.children[0]
		switch n.nType {
		case param:
			// 查找参数结尾（'/'或路径结尾）
			end := 0
			for end < len(path) && path[end] != '/' {
				end++
			}

			// 添加参数值到大小写不敏感路径中
			ciPath = append(ciPath, path[:end]...)

			// 继续处理剩余路径
			if end < len(path) {
				if len(n.children) > 0 {
					// 继续处理子节点
					n = n.children[0]
					npLen = len(n.path)
					path = path[end:]
					continue
				}

				// ... but we can't
				if fixTrailingSlash && len(path) == end+1 {
					return ciPath
				}
				return nil
			}

			if n.handlers != nil {
				return ciPath
			}

			if fixTrailingSlash && len(n.children) == 1 {
				// 没有找到处理函数。检查是否有处理此路径加结尾斜杠的处理函数
				n = n.children[0]
				if n.path == "/" && n.handlers != nil {
					return append(ciPath, '/')
				}
			}

			return nil

		case catchAll:
			// 对于catchAll节点，直接返回匹配的路径
			return append(ciPath, path...)

		default:
			panic("invalid node type")
		}
	}

	// 未找到任何匹配。尝试通过添加或删除结尾斜杠修正路径
	if fixTrailingSlash {
		if path == "/" {
			return ciPath
		}
		if len(path)+1 == npLen && n.path[len(path)] == '/' &&
			strings.EqualFold(path[1:], n.path[1:len(path)]) && n.handlers != nil {
			return append(ciPath, n.path...)
		}
	}
	return nil
}
