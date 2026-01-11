package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/c-bata/go-prompt"
)

var path string = "/"

// 支持的命令列表
var commands = []prompt.Suggest{
	{Text: "help", Description: "Display help"},
	{Text: "get", Description: "Get a key"},
	{Text: "set", Description: "Set a key-value pair"},
	{Text: "sub", Description: "Subscribe to a key"},
	{Text: "quit", Description: "Quit the application"},
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
		return
	case "path":
		if len(args) > 1 {
			path = args[1]
			fmt.Println("Changed path to:", path)
		} else {
			fmt.Println("Current path:", path)
		}
	case "help":
		fmt.Println("Available commands: help, get, set, sub, quit")
	case "get":
		if len(args) > 1 {
			fmt.Printf("Getting key: %s\n", args[1])
		} else {
			fmt.Println("Usage: get <key>")
		}
	case "set":
		if len(args) > 2 {
			fmt.Printf("Setting %s = %s\n", args[1], args[2])
		} else {
			fmt.Println("Usage: set <key> <value>")
		}
	case "sub":
		if len(args) > 1 {
			for {
				time.Sleep(1 * time.Second)
				fmt.Println("subscribe key: %s\n", args[1])

				c := make(chan os.Signal)
				signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

				sig := <-c
				fmt.Println("Received signal: %s", sig)
				break
			}
		} else {
			fmt.Println("Usage: sub <key>")
		}
	default:
		fmt.Printf("Unknown command: %s. Type 'help' for available commands.\n", cmd)
	}
}

// 提供自动补全建议
func completer(d prompt.Document) []prompt.Suggest {
	w := d.GetWordBeforeCursor()
	if w == "" {
		// 补全顶级命令
		return prompt.FilterHasPrefix(commands, w, true)
	}

	// 根据第一个词提供不同的补全
	first := strings.ToLower(strings.Fields(d.TextBeforeCursor())[0])
	switch first {
	case "get", "set", "sub":
		// 这里可以扩展参数补全逻辑
		break
	}

	// 简单上下文补全（这里只做命令补全，不深入参数）
	return prompt.FilterHasPrefix(commands, w, true)
}

func main() {
	fmt.Println("Welcome to MyPrompt! Type 'help' or 'quit'")

	p := prompt.New(
		executor,
		completer,
		prompt.OptionTitle("MyPrompt"),
		prompt.OptionPrefix(">>> "),
		prompt.OptionHistory([]string{"help", "get name"}),
	)

	p.Run()
}
