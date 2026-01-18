package main

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/c-bata/go-prompt"
	"github.com/openconfig/goyang/pkg/yang"
)

var path string = "/"
var modules = yang.NewModules()
var SchemaTree *yang.Entry

// 全局订阅控制变量
var (
	subscribeCancel chan bool
	isSubscribing   bool
)

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

// 订阅类型建议列表
var subscriptionTypeSuggestions = []prompt.Suggest{
	{Text: "sample", Description: "Default subscription key"},
	{Text: "on_change", Description: "Stream subscription"},
	{Text: "once", Description: "One-time subscription"},
}

func printSchemaTree(entry *yang.Entry, indent string) {
	fmt.Printf("%s%s (%s)\n", indent, entry.Name, entry.Kind.String())
	for _, child := range entry.Dir {
		printSchemaTree(child, indent+"  ")
	}
}

func help(buf *prompt.Buffer) {
	fmt.Println("Available commands:")
}

// 在init函数中添加（用于调试）
func init() {
	generateYangSchema("yang/example-telemetry.yang")
	// 打印schema树结构以调试
	fmt.Println("Schema Tree Structure:")
	printSchemaTree(SchemaTree, "")
}

func generateYangSchema(file string) error {
	if file == "" {
		return nil
	}

	if err := modules.Read(file); err != nil {
		return err
	}

	if errors := modules.Process(); len(errors) > 0 {
		for _, e := range errors {
			fmt.Fprintf(os.Stderr, "yang processing error: %v\n", e)
		}
		return fmt.Errorf("yang processing failed with %d errors", len(errors))
	}
	// Keep track of the top level modules we read in.
	// Those are the only modules we want to print below.
	mods := map[string]*yang.Module{}
	var names []string

	for _, m := range modules.Modules {
		if mods[m.Name] == nil {
			mods[m.Name] = m
			names = append(names, m.Name)
		}
	}
	sort.Strings(names)
	entries := make([]*yang.Entry, len(names))
	for x, n := range names {
		entries[x] = yang.ToEntry(mods[n])
	}

	SchemaTree = buildRootEntry()

	for _, entry := range entries {
		updateAnnotation(entry)
		SchemaTree.Dir[entry.Name] = entry
	}
	return nil
}

func buildRootEntry() *yang.Entry {
	return &yang.Entry{
		Name: "root",
		Kind: yang.DirectoryEntry,
		Dir:  make(map[string]*yang.Entry),
		Annotation: map[string]interface{}{
			"schemapath": "/",
			"root":       true,
		},
	}
}

func findXPathSuggestions(doc prompt.Document) []prompt.Suggest {
	word := doc.GetWordBeforeCursor()
	suggestions := make([]prompt.Suggest, 0, 16)
	// generate suggestions from yang schema
	for _, entry := range SchemaTree.Dir {
		suggestions = append(suggestions, findMatchedXPATH(entry, word, false)...)
	}

	sort.Slice(suggestions, func(i, j int) bool {
		if suggestions[i].Text == suggestions[j].Text {
			return suggestions[i].Description < suggestions[j].Description
		}
		return suggestions[i].Text < suggestions[j].Text
	})

	return suggestions
}

// updateAnnotation updates the schema info before encoding.
func updateAnnotation(entry *yang.Entry) {
	for _, child := range entry.Dir {
		updateAnnotation(child)
		child.Annotation = map[string]interface{}{}
		t := child.Type
		if t == nil {
			continue
		}

		switch t.Kind {
		case yang.Ybits:
			nameMap := t.Bit.NameMap()
			bits := make([]string, 0, len(nameMap))
			for bitstr := range nameMap {
				bits = append(bits, bitstr)
			}
			child.Annotation["bits"] = bits
		case yang.Yenum:
			nameMap := t.Enum.NameMap()
			enum := make([]string, 0, len(nameMap))
			for enumstr := range nameMap {
				enum = append(enum, enumstr)
			}
			child.Annotation["enum"] = enum
		case yang.Yidentityref:
			identities := make([]string, 0, len(t.IdentityBase.Values))
			for i := range t.IdentityBase.Values {
				identities = append(identities, t.IdentityBase.Values[i].PrefixedName())
			}
			child.Annotation["prefix-qualified-identities"] = identities
		}
		if t.Root != nil {
			child.Annotation["root.type"] = t.Root.Name
		}
	}
}

func findMatchedXPATH(entry *yang.Entry, input string, prefixPresent bool) []prompt.Suggest {
	if strings.HasPrefix(input, ":") {
		return nil
	}
	suggestions := make([]prompt.Suggest, 0, 4)
	inputLen := len(input)
	for i, c := range input {
		if c == ':' && i+1 < inputLen {
			input = input[i+1:]
			inputLen -= (i + 1)
			break
		}
	}

	prependOrigin := false
	for name, child := range entry.Dir {
		if child.IsCase() || child.IsChoice() {
			for _, gchild := range child.Dir {
				suggestions = append(suggestions, findMatchedXPATH(gchild, input, prefixPresent)...)
			}
			continue
		}
		pathelem := "/" + name
		if strings.HasPrefix(pathelem, input) {
			node := ""
			if inputLen == 0 && prependOrigin {
				node = fmt.Sprintf("%s:/%s", entry.Name, name)
			} else if inputLen > 0 && input[0] == '/' {
				node = name
			} else {
				node = pathelem
			}
			suggestions = append(suggestions, prompt.Suggest{Text: node, Description: buildXPATHDescription(child)})
			if child.Key != "" { // list
				keylist := strings.Split(child.Key, " ")
				for _, key := range keylist {
					node = fmt.Sprintf("%s[%s=*]", node, key)
				}
				suggestions = append(suggestions, prompt.Suggest{Text: node, Description: buildXPATHDescription(child)})
			}
		} else if strings.HasPrefix(input, pathelem) {
			var prevC rune
			var bracketCount int
			var endIndex int = -1
			var stop bool
			for i, c := range input {
				switch c {
				case '[':
					bracketCount++
				case ']':
					if prevC != '\\' {
						bracketCount--
						endIndex = i
					}
				case '/':
					if i != 0 && bracketCount == 0 {
						endIndex = i
						stop = true
					}
				}
				if stop {
					break
				}
				prevC = c
			}
			if bracketCount == 0 {
				if endIndex >= 0 {
					suggestions = append(suggestions, findMatchedXPATH(child, input[endIndex:], prefixPresent)...)
				} else {
					suggestions = append(suggestions, findMatchedXPATH(child, input[len(pathelem):], prefixPresent)...)
				}
			}
		}
	}
	return suggestions
}

func buildXPATHDescription(entry *yang.Entry) string {
	sb := strings.Builder{}
	sb.WriteString(getDescriptionPrefix(entry))
	sb.WriteString(" ")

	if entry.Type != nil {
		sb.WriteString(entry.Type.Kind.String())
		sb.WriteString(", ")
	}
	sb.WriteString(entry.Description)
	return sb.String()
}

func getDescriptionPrefix(entry *yang.Entry) string {
	switch {
	case entry.Dir == nil && entry.ListAttr != nil: // leaf-list
		return "[⋯]"
	case entry.Dir == nil: // leaf
		return "   "
	case entry.ListAttr != nil: // list
		return "[+]"
	default: // container
		return "[+]"
	}
}

func resetTerminal() {
	var cmd *exec.Cmd
	cmd = exec.Command("reset")
	cmd.Stdout = os.Stdout
	_ = cmd.Run() // 忽略错误，以防命令失败
}

// 停止订阅
func stopSubscription() {
	if subscribeCancel != nil {
		subscribeCancel <- true
		close(subscribeCancel)
		subscribeCancel = nil
		isSubscribing = false
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
		stopSubscription()
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
		// 停止之前的订阅
		stopSubscription()

		var subKey string
		if len(args) > 1 {
			subKey = args[1]
		} else {
			subKey = "sample"
		}

		// 创建新的取消通道
		subscribeCancel = make(chan bool, 1)
		isSubscribing = true

		// 在goroutine中启动订阅，避免阻塞executor
		go runSubscription(subKey)

		fmt.Printf("Subscription started with key: %s (press Ctrl+C to stop)\n", subKey)

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
	allFields := strings.Split(textBeforeCursor, " ")
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
			return prompt.FilterHasPrefix(subscriptionTypeSuggestions, "", true)
		case "path":
			// 对于path命令，可以提供根路径建议
			return findXPathSuggestions(d)
		}
		return []prompt.Suggest{}
	}

	switch first {
	case "get", "quit", "help":
		return []prompt.Suggest{}
	case "path":
		return findXPathSuggestions(d)
	case "set":
		if len(allFields) >= 3 {
			return []prompt.Suggest{}
		}
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
		if len(allFields) >= 3 {
			return []prompt.Suggest{}
		}
		if len(fields) == 2 {
			// 输入sub命令后的参数
			return prompt.FilterHasPrefix(subscriptionTypeSuggestions, lastWord, true)
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
		prompt.OptionAddASCIICodeBind(
			// bind '?' character to show cmd args
			prompt.ASCIICodeBind{
				ASCIICode: []byte{0x3f},
				Fn:        help,
			},
			// bind OS X Option+Left key binding
			prompt.ASCIICodeBind{
				ASCIICode: []byte{0x1b, 0x62},
				Fn:        prompt.GoLeftWord,
			},
			// bind OS X Option+Right key binding
			prompt.ASCIICodeBind{
				ASCIICode: []byte{0x1b, 0x66},
				Fn:        prompt.GoRightWord,
			},
		),
		prompt.OptionAddKeyBind(
			prompt.KeyBind{
				Key: prompt.ControlZ,
				Fn: func(buf *prompt.Buffer) {
					// If the last word before the cursor does not contain a "/" return.
					// This is needed to avoid deleting down to a previous flag value
					if !strings.Contains(buf.Document().GetWordBeforeCursorWithSpace(), "/") {
						return
					}
					// Check if the last rune is a PathSeparator and is not the path root then delete it
					if buf.Document().GetCharRelativeToCursor(0) == os.PathSeparator && buf.Document().GetCharRelativeToCursor(-1) != ' ' {
						buf.DeleteBeforeCursor(1)
					}
					// Delete down until the next "/"
					buf.DeleteBeforeCursor(len([]rune(buf.Document().GetWordBeforeCursorUntilSeparator("/"))))
				},
			},
			prompt.KeyBind{
				Key: prompt.ControlC,
				Fn: func(buf *prompt.Buffer) {
					// 检查是否有活跃的订阅
					if isSubscribing && subscribeCancel != nil {
						stopSubscription()
						fmt.Println("Subscription canceled...")
					} else {
						fmt.Println("Exiting...")
						resetTerminal()
						os.Exit(0)
					}
				},
			},
		),
		prompt.OptionCompletionWordSeparator(string([]byte{' ', os.PathSeparator})),
		prompt.OptionShowCompletionAtStart(),
	)

	p.Run()
}
