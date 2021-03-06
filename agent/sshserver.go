package main

import (
	"C"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	sshserver "github.com/gliderlabs/ssh"
	"github.com/kr/pty"
	"github.com/sirupsen/logrus"
)

import (
	"net"
)

type sshConn struct {
	net.Conn
	closeCallback func()
}

func (c *sshConn) Close() error {
	c.closeCallback()
	return c.Conn.Close()
}

type SSHServer struct {
	sshd       *sshserver.Server
	cmds       map[string]*exec.Cmd
	deviceName string
}

func NewSSHServer(privateKey string, port int) *SSHServer {
	s := &SSHServer{
		cmds: make(map[string]*exec.Cmd),
	}

	s.sshd = &sshserver.Server{
		Addr: fmt.Sprintf("localhost:%d", port),
		PasswordHandler: func(ctx sshserver.Context, pass string) bool {
			if Auth(ctx.User(), pass) == true {
				return true
			}

			return false
		},
		PublicKeyHandler: s.publicKeyHandler,
		Handler:          s.sessionHandler,
		ConnCallback: func(ctx sshserver.Context, conn net.Conn) net.Conn {
			closeCallback := func() {
				if v, ok := s.cmds[ctx.SessionID()]; ok {
					v.Process.Kill()
					delete(s.cmds, ctx.SessionID())
				}
			}

			return &sshConn{conn, closeCallback}
		},
	}

	s.sshd.SetOption(sshserver.HostKeyFile(privateKey))

	return s
}

func (s *SSHServer) ListenAndServe() error {
	return s.sshd.ListenAndServe()
}

func (s *SSHServer) SetDeviceName(name string) {
	s.deviceName = name
}

func (s *SSHServer) sessionHandler(session sshserver.Session) {
	sspty, winCh, isPty := session.Pty()

	if isPty {
		scmd := newShellCmd(s, session.User(), sspty.Term)

		spty, err := pty.Start(scmd)
		if err != nil {
			logrus.Warn(err)
		}

		go func() {
			for win := range winCh {
				setWinsize(spty, win.Width, win.Height)
			}
		}()

		go func() {
			_, err := io.Copy(session, spty)
			if err != nil {
				logrus.Warn(err)
			}
		}()

		go func() {
			_, err := io.Copy(spty, session)
			if err != nil {
				logrus.Warn(err)
			}
		}()

		s.cmds[session.Context().Value(sshserver.ContextKeySessionID).(string)] = scmd

		err = scmd.Wait()
		if err != nil {
			logrus.Warn(err)
		}
	} else {
		u := lookupUser(session.User())
		cmd := newCmd(u, "", "", s.deviceName, session.Command()...)

		stdout, _ := cmd.StdoutPipe()
		stdin, _ := cmd.StdinPipe()

		cmd.Start()

		go func() {
			if _, err := io.Copy(stdin, session); err != nil {
				fmt.Println(err)
			}
		}()

		go func() {
			if _, err := io.Copy(session, stdout); err != nil {
				fmt.Println(err)
			}
		}()

		cmd.Wait()
	}
}

func (s *SSHServer) publicKeyHandler(ctx sshserver.Context, key sshserver.PublicKey) bool {
	return true
}

func newShellCmd(s *SSHServer, username string, term string) *exec.Cmd {
	shell := os.Getenv("SHELL")

	u := lookupUser(username)

	if shell == "" {
		shell = u.Shell
	}

	if term == "" {
		term = "xterm"
	}

	cmd := newCmd(u, shell, term, s.deviceName, shell, "--login")

	return cmd
}

func setWinsize(f *os.File, w, h int) {
	size := &struct{ h, w, x, y uint16 }{uint16(h), uint16(w), 0, 0}
	syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(syscall.TIOCSWINSZ), uintptr(unsafe.Pointer(size)))
}
