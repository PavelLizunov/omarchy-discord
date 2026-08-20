// Command omarchy-discord-backend is the Go daemon behind the quickshell.discord
// plugin. Subcommands:
//
//	serve   (default) run the socket server and Discord session
//	check   print a JSON environment summary and exit
//	login   read a token from stdin, validate it, store it in the keyring
//	logout  clear the keyring entry
//
// Exit codes are listed in backend/README.md.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/diamondburned/ningen/v3"

	"github.com/mattcalayo/omarchy-discord/backend/internal/keyring"
	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
	"github.com/mattcalayo/omarchy-discord/backend/internal/redact"
	"github.com/mattcalayo/omarchy-discord/backend/internal/session"
	"github.com/mattcalayo/omarchy-discord/backend/internal/socket"
)

// Exit codes. Distinct per failure so scripts can tell them apart.
const (
	exitOK            = 0
	exitUsage         = 1
	exitRuntimeDir    = 2 // runtime dir not writable / socket bind failed
	exitNoSecretTool  = 3
	exitNoToken       = 4 // check: no token stored; login: empty stdin
	exitLoginRejected = 5
	exitKeyring       = 6 // secret-tool store/clear failed
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("omarchy-discord-backend", flag.ContinueOnError)
	socketPath := fs.String("socket-path", socket.DefaultPath(), "unix socket path")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: omarchy-discord-backend [--socket-path PATH] [serve|check|login|logout]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	cmd := "serve"
	if fs.NArg() > 0 {
		cmd = fs.Arg(0)
	}
	// Flags may also follow the subcommand.
	if fs.NArg() > 1 {
		if err := fs.Parse(fs.Args()[1:]); err != nil {
			return exitUsage
		}
	}
	switch cmd {
	case "serve":
		return serve(*socketPath)
	case "check":
		return check(*socketPath)
	case "login":
		return login()
	case "logout":
		return logout()
	}
	fs.Usage()
	return exitUsage
}

func serve(socketPath string) int {
	session.ConfigureIdentity()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if !keyring.Available() {
		redact.Logf("secret-tool not found; login will not persist")
	}
	mgr := session.New(keyring.Keyring{})
	srv := socket.New(socketPath, mgr)
	if err := srv.Listen(); err != nil {
		redact.Logf("%v", err)
		return exitRuntimeDir
	}
	redact.Logf("listening on %s (backend %s, protocol v%d)", socketPath, protocol.BackendVersion, protocol.Version)

	go func() {
		for ev := range mgr.Events() {
			srv.Broadcast(ev)
		}
	}()
	go mgr.Start(ctx)

	srv.Serve(ctx)
	mgr.Stop()
	redact.Logf("stopped")
	return exitOK
}

type checkReport struct {
	OK                 bool   `json:"ok"`
	BackendVersion     string `json:"backend_version"`
	ProtocolVersion    int    `json:"protocol_version"`
	SocketPath         string `json:"socket_path"`
	RuntimeDir         string `json:"runtime_dir"`
	RuntimeDirWritable bool   `json:"runtime_dir_writable"`
	SecretTool         bool   `json:"secret_tool"`
	TokenPresent       bool   `json:"token_present"`
	Error              string `json:"error,omitempty"`
}

func check(socketPath string) int {
	r := checkReport{BackendVersion: protocol.BackendVersion, ProtocolVersion: protocol.Version, SocketPath: socketPath}
	r.RuntimeDir = filepath.Dir(socketPath)
	r.RuntimeDirWritable = dirWritable(r.RuntimeDir)
	r.SecretTool = keyring.Available()
	code := exitOK
	if r.SecretTool {
		_, err := keyring.Keyring{}.Lookup(context.Background())
		switch {
		case err == nil:
			r.TokenPresent = true
		case errors.Is(err, keyring.ErrNotFound):
		default:
			r.Error = redact.Redact(err.Error())
		}
	}
	switch {
	case !r.RuntimeDirWritable:
		code = exitRuntimeDir
	case !r.SecretTool:
		code = exitNoSecretTool
	case !r.TokenPresent:
		code = exitNoToken
	}
	r.OK = code == exitOK
	enc := json.NewEncoder(os.Stdout)
	enc.Encode(r)
	return code
}

// dirWritable creates dir (0700) if needed and probes it with a temp file.
func dirWritable(dir string) bool {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return false
	}
	f, err := os.CreateTemp(dir, ".probe-*")
	if err != nil {
		return false
	}
	f.Close()
	os.Remove(f.Name())
	return true
}

// login reads a token from stdin (first line), validates it with REST
// /users/@me, and stores it in the keyring.
func login() int {
	if !keyring.Available() {
		fmt.Fprintln(os.Stderr, "secret-tool not found")
		return exitNoSecretTool
	}
	session.ConfigureIdentity()
	line, err := bufio.NewReader(io.LimitReader(os.Stdin, 4096)).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		fmt.Fprintln(os.Stderr, "cannot read token from stdin")
		return exitUsage
	}
	token := strings.TrimSpace(line)
	if token == "" {
		fmt.Fprintln(os.Stderr, "no token on stdin")
		return exitNoToken
	}
	n := ningen.New(token)
	me, err := n.Me()
	if err != nil {
		fmt.Fprintln(os.Stderr, "token rejected:", redact.Redact(err.Error()))
		return exitLoginRejected
	}
	if err := (keyring.Keyring{}).Store(context.Background(), token); err != nil {
		fmt.Fprintln(os.Stderr, redact.Redact(err.Error()))
		return exitKeyring
	}
	fmt.Printf("stored token for %s (%s)\n", me.Username, me.ID)
	return exitOK
}

func logout() int {
	if !keyring.Available() {
		fmt.Fprintln(os.Stderr, "secret-tool not found")
		return exitNoSecretTool
	}
	if err := (keyring.Keyring{}).Clear(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, redact.Redact(err.Error()))
		return exitKeyring
	}
	fmt.Println("keyring entry cleared")
	return exitOK
}
