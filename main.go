package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/c-bata/go-prompt"
)

var path string = "/"

// 全局订阅控制变量
var (
	subscribeCancel chan bool
)

// 模拟YANG模块的结构
type YangNode struct {
	Name        string
	Description string
	Type        string
	Children    map[string]*YangNode
}

// 预定义的YANG模块结构
var yangModules = map[string]*YangNode{
	"test-module": {
		Name:        "test-module",
		Description: "Test YANG module for demonstration",
		Children: map[string]*YangNode{
			"interfaces": {
				Name:        "interfaces",
				Description: "Network interfaces container",
				Children: map[string]*YangNode{
					"interface": {
						Name:        "interface",
						Description: "List of interfaces",
						Type:        "list",
						Children: map[string]*YangNode{
							"name": {
								Name:        "name",
								Description: "Interface name",
								Type:        "string",
							},
							"enabled": {
								Name:        "enabled",
								Description: "Interface enable status",
								Type:        "boolean",
							},
						},
					},
				},
			},
			"system": {
				Name:        "system",
				Description: "System container",
				Children: map[string]*YangNode{
					"hostname": {
						Name:        "hostname",
						Description: "Device hostname",
						Type:        "string",
					},
					"servers": {
						Name:        "servers",
						Description: "List of servers",
						Type:        "leaf-list",
					},
				},
			},
			"routing": {
				Name:        "routing",
				Description: "Routing configuration",
				Children: map[string]*YangNode{
					"static-routes": {
						Name:        "static-routes",
						Description: "Static routing table",
						Children: map[string]*YangNode{
							"route": {
								Name:        "route",
								Description: "A static route",
								Type:        "list",
								Children: map[string]*YangNode{
									"destination": {
										Name:        "destination",
										Description: "Route destination",
										Type:        "string",
									},
									"next-hop": {
										Name:        "next-hop",
										Description: "Next hop address",
										Type:        "string",
									},
								},
							},
						},
					},
				},
			},
		},
	},
}

// 支持的命令列表
var commands = []prompt.Suggest{
	{Text: "help", Description: "Display help information"},
	{Text: "get", Description: "Execute gNMI GetRequest"},
	{Text: "path", Description: "Set gNMI path"},
	{Text: "set", Description: "Execute gNMI SetRequest"},
	{Text: "sub", Description: "Execute gNMI SubscribeRequest"},
	{Text: "quit", Description: "Quit the application"},
}

// 数据类型建议列表
var dataTypeSuggestions = []prompt.Suggest{
	{Text: "json", Description: "JSON encoded string (RFC7159)"},
	{Text: "bytes", Description: "Byte sequence whose semantics is opaque to the protocol"},
	{Text: "proto", Description: "Serialised protobuf message using protobuf.Any"},
	{Text: "ascii", Description: "ASCII encoded string representing text formatted according to a target-defined convention"},
	{Text: "json_ietf", Description: "JSON_IETF encoded string (RFC7951)"},
}

func resetTerminal() {
	var cmd *exec.Cmd
	cmd = exec.Command("reset")
	cmd.Stdout = os.Stdout
	_ = cmd.Run() // 忽略错误，以防命令失败
}

// 从YANG模块结构中构建XPath建议
func buildXPathSuggestions(input string) []prompt.Suggest {
	var suggestions []prompt.Suggest

	if input == "" {
		input = "/"
	}

	// 确保输入以/开头
	if !strings.HasPrefix(input, "/") {
		input = "/" + input
	}

	// 获取所有可能的XPath
	for _, module := range yangModules {
		suggestions = append(suggestions, getNodePaths(module, input)...)
	}

	return suggestions
}

// 递归获取节点路径
func getNodePaths(node *YangNode, prefix string) []prompt.Suggest {
	var suggestions []prompt.Suggest

	if node == nil {
		return suggestions
	}

	// 跳过根模块节点
	if node.Name == "test-module" {
		// 遍历子节点
		for _, child := range node.Children {
			suggestions = append(suggestions, getChildPaths(child, "", prefix)...)
		}
	} else {
		suggestions = append(suggestions, getChildPaths(node, "", prefix)...)
	}

	return suggestions
}

// 获取子节点路径
func getChildPaths(node *YangNode, currentPath, prefix string) []prompt.Suggest {
	var suggestions []prompt.Suggest

	// 构建当前节点的完整路径
	fullPath := currentPath
	if currentPath == "" {
		fullPath = "/" + node.Name
	} else {
		fullPath = currentPath + "/" + node.Name
	}

	// 如果当前路径匹配前缀，添加到建议列表
	if strings.HasPrefix(fullPath, prefix) || prefix == "" {
		suggestions = append(suggestions, prompt.Suggest{
			Text:        fullPath,
			Description: node.Description,
		})
	}

	// 递归处理子节点
	if node.Children != nil {
		for _, child := range node.Children {
			suggestions = append(suggestions, getChildPaths(child, fullPath, prefix)...)
		}
	}

	return suggestions
}

// 停止订阅
func stopSubscription() {
	if subscribeCancel != nil {
		subscribeCancel <- true
		close(subscribeCancel)
		subscribeCancel = nil
	}
}

// 执行订阅操作
func runSubscription(subKey string) {
	fmt.Printf("Starting subscription for key: %s, path: %s\n", subKey, path)

	// 模拟订阅
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-subscribeCancel:
			fmt.Println("Subscription stopped")
			return
		case <-ticker.C:
			fmt.Printf("Received update for subscription '%s': %s\n", subKey, path)
		}
	}
}

// 执行用户输入的命令
func executor(in string) {
	in = strings.TrimSpace(in)
	if in == "" {
		return
	}

	args := strings.Fields(in)
	cmd := args[0]

	switch cmd {
	case "quit":
		fmt.Println("Goodbye!")
		resetTerminal()
		os.Exit(0)
	case "path":
		if len(args) > 1 {
			path = args[1]
			fmt.Println("Changed path to:", path)
		} else {
			fmt.Println("Current path:", path)
		}
	case "help":
		fmt.Println("Available gNMI commands:")
		fmt.Println("- help: Display help information")
		fmt.Println("- get: Execute gNMI GetRequest for current path")
		fmt.Println("- set <type> <value>: Execute gNMI SetRequest with specified data type and value")
		fmt.Println("- sub [key]: Execute gNMI SubscribeRequest, default key is 'sample'")
		fmt.Println("- path: Show or set gNMI path")
		fmt.Println("- quit: Quit the application")
	case "get":
		fmt.Printf("Executing gNMI GetRequest for path: %s\n", path)
	case "set":
		if len(args) >= 3 {
			dataType := args[1]
			value := args[2]
			fmt.Printf("Executing gNMI SetRequest - Path: %s, Type: %s, Value: %s\n", path, dataType, value)
		} else {
			fmt.Println("Usage: set <type> <value>")
			fmt.Println("Supported types: json, bytes, proto, ascii, json_ietf")
		}
	case "sub":
		var subKey string
		if len(args) > 1 {
			subKey = args[1]
		} else {
			subKey = "sample"
		}

		// 创建新的取消通道
		subscribeCancel = make(chan bool, 1)

		// 启动订阅（在后台运行）
		runSubscription(subKey)

		fmt.Printf("Subscription started with key: %s\n", subKey)

	default:
		fmt.Printf("Unknown command: %s. Type 'help' for available commands.\n", cmd)
	}
}

// 提供自动补全建议
func completer(d prompt.Document) []prompt.Suggest {
	textBeforeCursor := d.TextBeforeCursor()
	if strings.TrimSpace(textBeforeCursor) == "" {
		return []prompt.Suggest{}
	}

	fields := strings.Fields(textBeforeCursor)
	if len(fields) == 0 {
		return prompt.FilterHasPrefix(commands, d.GetWordBeforeCursor(), true)
	}

	first := strings.ToLower(fields[0])
	lastWord := d.GetWordBeforeCursor()

	// 如果当前正在输入命令本身
	if len(fields) == 1 && strings.HasSuffix(textBeforeCursor, " ") {
		// 刚刚输入完命令并按下空格，根据命令提供参数建议
		switch first {
		case "set":
			return prompt.FilterHasPrefix(dataTypeSuggestions, "", true)
		case "sub":
			return []prompt.Suggest{{Text: "sample", Description: "Default subscription key"}}
		case "path":
			// 对于path命令，可以提供根路径建议
			return buildXPathSuggestions("")
		}
		return []prompt.Suggest{}
	}

	switch first {
	case "get", "quit", "help":
		return []prompt.Suggest{}
	case "path":
		// 为path命令提供路径建议
		if len(fields) > 1 {
			// 获取当前输入的路径部分
			pathInput := lastWord
			if !strings.HasPrefix(pathInput, "/") && len(fields) > 1 {
				// 如果当前单词不以/开头，可能是一个路径的部分
				pathInput = fields[1]
				if len(fields) > 2 && fields[len(fields)-1] != "" {
					// 有多个路径部分，重建路径
					pathInput = strings.Join(fields[1:], "/")
				}
			}
			return buildXPathSuggestions(pathInput)
		}
		return buildXPathSuggestions("")
	case "set":
		if len(fields) == 2 {
			// 正在输入数据类型
			return prompt.FilterHasPrefix(dataTypeSuggestions, lastWord, true)
		} else if len(fields) == 3 {
			// 正在输入值，不提供建议
			return []prompt.Suggest{}
		} else if len(fields) == 1 {
			// 只输入了set，还没按空格
			return prompt.FilterContains(dataTypeSuggestions, lastWord, true)
		}
		return []prompt.Suggest{}
	case "sub":
		if len(fields) == 2 {
			// 输入sub命令后的参数
			return prompt.FilterHasPrefix([]prompt.Suggest{
				{Text: "sample", Description: "Default subscription key"},
				{Text: "on_change", Description: "Stream subscription"},
				{Text: "once", Description: "One-time subscription"},
			}, lastWord, true)
		}
		return []prompt.Suggest{}
	default:
		// 检查是否在输入命令
		return prompt.FilterHasPrefix(commands, lastWord, true)
	}
}

func main() {
	fmt.Println("Welcome to gNMI CLI! Type 'help' or 'quit'")

	p := prompt.New(
		executor,
		completer,
		prompt.OptionTitle("gNMI CLI"),
		prompt.OptionPrefix("gNMI> "),
		prompt.OptionHistory([]string{"help", "get", "path"}),
		prompt.OptionAddKeyBind(
			prompt.KeyBind{
				Key: prompt.ControlZ,
				Fn: func(buf *prompt.Buffer) {
					// 获取光标前的单词
					word := buf.Document().GetWordBeforeCursor()

					// 如果单词包含/，则删除直到前一个/
					if strings.Contains(word, "/") {
						// 找到最后一个/的位置
						lastSlash := strings.LastIndex(word, "/")
						if lastSlash > 0 {
							// 删除从光标位置到最后一个/之后的内容
							toDelete := len(word) - lastSlash - 1
							if toDelete > 0 {
								buf.DeleteBeforeCursor(toDelete)
							}
						} else if lastSlash == 0 {
							// 单词以/开头，删除/之后的所有内容
							buf.DeleteBeforeCursor(len(word) - 1)
						}
					} else {
						// 删除整个单词
						buf.DeleteBeforeCursor(len([]rune(word)))
					}
				},
			},
			prompt.KeyBind{
				Key: prompt.ControlC,
				Fn: func(buf *prompt.Buffer) {
					// 检查是否有活跃的订阅
					fmt.Println("ctrl+c pressed")
					if subscribeCancel != nil {
						stopSubscription()
						fmt.Println("\nSubscription canceled...")
					} else {
						fmt.Println("\nExiting...")
						resetTerminal()
						os.Exit(0)
					}
				},
			},
		),
	)

	p.Run()
}
