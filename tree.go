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

// nodeValue holds return values of (*Node).getValue method
type nodeValue struct {
	handlers HandlersChain
	params   *Params
	tsr      bool
	fullPath string
}

type skippedNode struct {
	path        string
	node        *node
	paramsCount int16
}

// Returns the handle registered with the given path (key). The values of
// wildcards are saved to a map.
// If no handle can be found, a TSR (trailing slash redirect) recommendation is
// made if a handle exists with an extra (without the) trailing slash for the
// given path.
func (n *node) getValue(path string, params *Params, skippedNodes *[]skippedNode, unescape bool) (value nodeValue) {
	var globalParamsCount int16

walk: // Outer loop for walking the tree
	for {
		prefix := n.path
		if len(path) > len(prefix) {
			if path[:len(prefix)] == prefix {
				path = path[len(prefix):]

				// Try all the non-wildcard children first by matching the indices
				idxc := path[0]
				for i, c := range []byte(n.indices) {
					if c == idxc {
						//  strings.HasPrefix(n.children[len(n.children)-1].path, ":") == n.wildChild
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

						n = n.children[i]
						continue walk
					}
				}

				if !n.wildChild {
					// If the path at the end of the loop is not equal to '/' and the current node has no child nodes
					// the current node needs to roll back to last valid skippedNode
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

					// Nothing found.
					// We can recommend to redirect to the same URL without a
					// trailing slash if a leaf exists for that path.
					value.tsr = path == "/" && n.handlers != nil
					return
				}

				// Handle wildcard child, which is always at the end of the array
				n = n.children[len(n.children)-1]
				globalParamsCount++

				switch n.nType {
				case param:
					// fix truncate the parameter
					// tree_test.go  line: 204

					// Find param end (either '/' or path end)
					end := 0
					for end < len(path) && path[end] != '/' {
						end++
					}

					// Save param value
					if params != nil && cap(*params) > 0 {
						if value.params == nil {
							value.params = params
						}
						// Expand slice within preallocated capacity
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

					// we need to go deeper!
					if end < len(path) {
						if len(n.children) > 0 {
							path = path[end:]
							n = n.children[0]
							continue walk
						}

						// ... but we can't
						value.tsr = len(path) == end+1
						return
					}

					if value.handlers = n.handlers; value.handlers != nil {
						value.fullPath = n.fullPath
						return
					}
					if len(n.children) == 1 {
						// No handle found. Check if a handle for this path + a
						// trailing slash exists for TSR recommendation
						n = n.children[0]
						value.tsr = (n.path == "/" && n.handlers != nil) || (n.path == "" && n.indices == "/")
					}
					return

				case catchAll:
					// Save param value
					if params != nil {
						if value.params == nil {
							value.params = params
						}
						// Expand slice within preallocated capacity
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

					value.handlers = n.handlers
					value.fullPath = n.fullPath
					return

				default:
					panic("invalid node type")
				}
			}
		}

		if path == prefix {
			// If the current path does not equal '/' and the node does not have a registered handle and the most recently matched node has a child node
			// the current node needs to roll back to last valid skippedNode
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
				//	n = latestNode.children[len(latestNode.children)-1]
			}
			// We should have reached the node containing the handle.
			// Check if this node has a handle registered.
			if value.handlers = n.handlers; value.handlers != nil {
				value.fullPath = n.fullPath
				return
			}

			// If there is no handle for this route, but this route has a
			// wildcard child, there must be a handle for this path with an
			// additional trailing slash
			if path == "/" && n.wildChild && n.nType != root {
				value.tsr = true
				return
			}

			if path == "/" && n.nType == static {
				value.tsr = true
				return
			}

			// No handle found. Check if a handle for this path + a
			// trailing slash exists for trailing slash recommendation
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

		// Nothing found. We can recommend to redirect to the same URL with an
		// extra trailing slash if a leaf exists for that path
		value.tsr = path == "/" ||
			(len(prefix) == len(path)+1 && prefix[len(path)] == '/' &&
				path == prefix[:len(prefix)-1] && n.handlers != nil)

		// roll back to last valid skippedNode
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

// Makes a case-insensitive lookup of the given path and tries to find a handler.
// It can optionally also fix trailing slashes.
// It returns the case-corrected path and a bool indicating whether the lookup
// was successful.
func (n *node) findCaseInsensitivePath(path string, fixTrailingSlash bool) ([]byte, bool) {
	const stackBufSize = 128

	// Use a static sized buffer on the stack in the common case.
	// If the path is too long, allocate a buffer on the heap instead.
	buf := make([]byte, 0, stackBufSize)
	if length := len(path) + 1; length > stackBufSize {
		buf = make([]byte, 0, length)
	}

	ciPath := n.findCaseInsensitivePathRec(
		path,
		buf,       // Preallocate enough memory for new path
		[4]byte{}, // Empty rune buffer
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

// Recursive case-insensitive lookup function used by n.findCaseInsensitivePath
func (n *node) findCaseInsensitivePathRec(path string, ciPath []byte, rb [4]byte, fixTrailingSlash bool) []byte {
	npLen := len(n.path)

walk: // Outer loop for walking the tree
	for len(path) >= npLen && (npLen == 0 || strings.EqualFold(path[1:npLen], n.path[1:])) {
		// Add common prefix to result
		oldPath := path
		path = path[npLen:]
		ciPath = append(ciPath, n.path...)

		if len(path) == 0 {
			// We should have reached the node containing the handle.
			// Check if this node has a handle registered.
			if n.handlers != nil {
				return ciPath
			}

			// No handle found.
			// Try to fix the path by adding a trailing slash
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

		// If this node does not have a wildcard (param or catchAll) child,
		// we can just look up the next child node and continue to walk down
		// the tree
		if !n.wildChild {
			// 跳过已经处理byte
			rb = shiftNRuneBytes(rb, npLen)

			if rb[0] != 0 {
				// Old rune not finished
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
				// Process a new rune
				var rv rune

				// Find rune start.
				// Runes are up to 4 byte long,
				// -4 would definitely be another rune.
				var off int
				for max := min(npLen, 3); off < max; off++ {
					if i := npLen - off; utf8.RuneStart(oldPath[i]) {
						// read rune from cached path
						rv, _ = utf8.DecodeRuneInString(oldPath[i:])
						break
					}
				}

				// Calculate lowercase bytes of current rune
				lo := unicode.ToLower(rv)
				utf8.EncodeRune(rb[:], lo)

				// Skip already processed bytes
				rb = shiftNRuneBytes(rb, off)

				idxc := rb[0]
				for i, c := range []byte(n.indices) {
					// Lowercase matches
					if c == idxc {
						// must use a recursive approach since both the
						// uppercase byte and the lowercase byte might exist
						// as an index
						if out := n.children[i].findCaseInsensitivePathRec(
							path, ciPath, rb, fixTrailingSlash,
						); out != nil {
							return out
						}
						break
					}
				}

				// If we found no match, the same for the uppercase rune,
				// if it differs
				if up := unicode.ToUpper(rv); up != lo {
					utf8.EncodeRune(rb[:], up)
					rb = shiftNRuneBytes(rb, off)

					idxc := rb[0]
					for i, c := range []byte(n.indices) {
						// Uppercase matches
						if c == idxc {
							// Continue with child node
							n = n.children[i]
							npLen = len(n.path)
							continue walk
						}
					}
				}
			}

			// Nothing found. We can recommend to redirect to the same URL
			// without a trailing slash if a leaf exists for that path
			if fixTrailingSlash && path == "/" && n.handlers != nil {
				return ciPath
			}
			return nil
		}

		n = n.children[0]
		switch n.nType {
		case param:
			// Find param end (either '/' or path end)
			end := 0
			for end < len(path) && path[end] != '/' {
				end++
			}

			// Add param value to case insensitive path
			ciPath = append(ciPath, path[:end]...)

			// We need to go deeper!
			if end < len(path) {
				if len(n.children) > 0 {
					// Continue with child node
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
				// No handle found. Check if a handle for this path + a
				// trailing slash exists
				n = n.children[0]
				if n.path == "/" && n.handlers != nil {
					return append(ciPath, '/')
				}
			}

			return nil

		case catchAll:
			return append(ciPath, path...)

		default:
			panic("invalid node type")
		}
	}

	// Nothing found.
	// Try to fix the path by adding / removing a trailing slash
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
