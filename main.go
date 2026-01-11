package main

import (
	"fmt"
	"strings"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/c-bata/go-prompt"
)

var path string = "/"

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

// 从YANG模块结构中构建XPath建议
func buildXPathSuggestions(input string) []prompt.Suggest {
	var suggestions []prompt.Suggest

	// 遍历所有模块
	for _, module := range yangModules {
		// 从根级别开始查找匹配的节点
		suggestions = append(suggestions, findMatchingNodes(module.Children, input)...)
	}

	return suggestions
}

// 递归查找匹配输入的节点
func findMatchingNodes(nodes map[string]*YangNode, input string) []prompt.Suggest {
	var suggestions []prompt.Suggest

	for _, node := range nodes {
		// 检查当前节点名称是否匹配输入
		fullPath := "/" + node.Name
		if strings.HasPrefix(fullPath, input) {
			description := node.Description
			if description == "" {
				description = "YANG node: " + node.Name
			}

			suggestions = append(suggestions, prompt.Suggest{
				Text:        fullPath,
				Description: description,
			})

			// 如果当前节点还有子节点，也考虑添加
			if node.Children != nil && len(node.Children) > 0 {
				// 如果输入完全匹配当前节点，还应提供子节点的建议
				if input == "/"+node.Name || strings.HasPrefix(input, "/"+node.Name+"/") {
					// 递归查找子节点
					childInput := strings.TrimPrefix(input, "/"+node.Name)
					if childInput == "" {
						childInput = "/"
					} else if childInput == "/" {
						childInput = ""
					}
					suggestions = append(suggestions, findMatchingNodes(node.Children, childInput)...)
				}
			}
		}

		// 如果输入是子路径的一部分，例如 "/interfaces/" 并且有子节点 "interface"
		if strings.HasPrefix(input, "/"+node.Name+"/") && node.Children != nil {
			childInput := strings.TrimPrefix(input, "/"+node.Name+"/")
			suggestions = append(suggestions, findMatchingChildNodes(node.Children, childInput, "/"+node.Name)...)
		}
	}

	return suggestions
}

// 查找匹配的子节点
func findMatchingChildNodes(nodes map[string]*YangNode, input string, parentPath string) []prompt.Suggest {
	var suggestions []prompt.Suggest

	for _, node := range nodes {
		fullPath := parentPath + "/" + node.Name
		if strings.HasPrefix(node.Name, input) || strings.HasPrefix(fullPath, input) {
			description := node.Description
			if description == "" {
				description = "YANG node: " + node.Name
			}

			suggestions = append(suggestions, prompt.Suggest{
				Text:        fullPath,
				Description: description,
			})
		}

		// 递归检查子节点
		if node.Children != nil && len(node.Children) > 0 {
			if strings.HasPrefix(input, node.Name+"/") {
				childInput := strings.TrimPrefix(input, node.Name+"/")
				suggestions = append(suggestions, findMatchingChildNodes(node.Children, childInput, parentPath+"/"+node.Name)...)
			}
		}
	}

	return suggestions
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

		// 创建一个信号通道用于接收中断信号
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

		// 启动一个goroutine来处理订阅逻辑
		doneChan := make(chan bool, 1)

		go func() {
			for {
				// 检查是否有退出信号
				select {
				case <-doneChan:
					return
				default:
					// 模拟订阅操作
					fmt.Printf("Executing gNMI SubscribeRequest for key: %s, path: %s, waiting for updates...\n", subKey, path)
					time.Sleep(2 * time.Second) // 模拟等待消息
				}
			}
		}()

		// 等待系统信号
		sig := <-sigChan
		fmt.Printf("\nReceived signal: %s, exiting subscription...\n", sig.String())

		// 发送完成信号，退出goroutine
		doneChan <- true

	default:
		fmt.Printf("Unknown command: %s. Type 'help' for available commands.\n", cmd)
	}
}

// 提供自动补全建议
func completer(d prompt.Document) []prompt.Suggest {
	w := d.GetWordBeforeCursor()

	// 根据第一个词提供不同的补全
	fields := strings.Fields(d.TextBeforeCursor())

	// 获取完整输入并分割，但保留最后一个空格输入
	textBeforeCursor := d.TextBeforeCursor()
	allFields := strings.Split(textBeforeCursor, " ")

	if len(fields) == 0 {
		// 没有任何输入时，显示命令补全
		return prompt.FilterHasPrefix(commands, w, true)
	}

	first := strings.ToLower(fields[0])
	switch first {
	case "get", "quit", "help":
		// 这些命令没有参数
		return []prompt.Suggest{}
	case "path":
		// 如果path命令后面有输入，则提供XPath建议
		if len(fields) == 1 && w == "" {
			// 用户只输入了"path"，还没有输入空格
			return prompt.FilterHasPrefix(commands, w, true)
		} else if len(fields) >= 1 {
			// 用户已经开始输入路径，提供基于YANG模块的建议
			return buildXPathSuggestions(w)
		}
		return []prompt.Suggest{}
	case "set":
		// 如果输入超过3个字段（命令+2个参数），说明已经在第三个参数之后输入了空格，不应再提示类型
		if len(fields) >= 3 {
			return []prompt.Suggest{}
		}
		// 检查是否刚输入set并按下了空格，或者正在输入第一个参数（数据类型）
		if len(allFields) == 2 && allFields[0] == "set" { // 刚输入set + 空格，等待数据类型
			// 在这种情况下，w 是空的，所以我们要显示所有数据类型
			return prompt.FilterHasPrefix(dataTypeSuggestions, w, true)
		} else if len(fields) == 2 && fields[0] == "set" { // 已经输入set和数据类型，正在输入值
			// 不再显示数据类型提示，因为用户应该输入值
			return []prompt.Suggest{}
		} else if len(fields) == 1 { // 只输入了set，还没按空格
			return prompt.FilterHasPrefix(dataTypeSuggestions, w, true)
		}
		return []prompt.Suggest{}
	case "sub":
		// 如果输入超过2个字段（命令+1个参数），说明已经在第二个参数之后输入了空格，不应再提示
		if len(allFields) >= 3 {
			return []prompt.Suggest{}
		}
		// 检查是否刚输入sub并按下了空格
		if len(allFields) == 2 && allFields[0] == "sub" && allFields[1] == "" {
			return prompt.FilterHasPrefix([]prompt.Suggest{{Text: "sample", Description: "Default subscription key"}}, w, true)
		}
		// 为sub命令提供补全
		if len(fields) == 2 {
			return prompt.FilterHasPrefix([]prompt.Suggest{{Text: "sample", Description: "Default subscription key"}}, w, true)
		}
		return []prompt.Suggest{}
	default:
		// 如果不是已知命令，仍然提供基本命令补全
		return prompt.FilterHasPrefix(commands, w, true)
	}
}

func main() {
	fmt.Println("Welcome to gNMI CLI! Type 'help' or 'quit'")

	prompt.New(
		executor,
		completer,
		prompt.OptionTitle("gNMI CLI"),
		prompt.OptionPrefix("gNMI> "),
		prompt.OptionHistory([]string{"help", "get", "path"}),
	).Run()
}