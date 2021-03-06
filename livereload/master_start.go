//+build !production

package livereload

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func (m *master) start(executablePath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.startingWorker != nil {
		m.startingWorker.Process.Kill()
		m.startingWorker = nil
	}

	worker := exec.Command(executablePath, os.Args[1:]...)
	worker.Env = append(worker.Env, os.Environ()...)
	switch m.config.Network {
	case "tcp", "tcp4", "tcp6":
		if strings.HasPrefix(m.config.Address, ":") {
			worker.Env = append(
				worker.Env,
				MasterEnv+"="+m.config.Network+"://localhost"+m.config.Address+"/",
			)
		} else if strings.HasPrefix(m.config.Address, "0.0.0.0:") {
			worker.Env = append(
				worker.Env,
				MasterEnv+"="+m.config.Network+"://localhost"+m.config.Address[7:]+"/",
			)
		} else if strings.HasPrefix(m.config.Address, "[::]:") {
			worker.Env = append(
				worker.Env,
				MasterEnv+"="+m.config.Network+"://localhost"+m.config.Address[4:]+"/",
			)
		} else {
			worker.Env = append(
				worker.Env,
				MasterEnv+"="+m.config.Network+"://"+m.config.Address+"/",
			)
		}
	case "unix", "unixpacket":
		path := strings.Replace(m.config.Address, string(os.PathSeparator), "/", -1)
		if !strings.HasPrefix(path, "/") {
			path = "/./" + path
		}
		worker.Env = append(
			worker.Env,
			MasterEnv+"="+"unix://"+path,
		)
	default:
		panic("cannot happen, net.Listen() would've failed")
	}
	worker.Env = append(worker.Env, ConfigHashEnv+"="+strconv.FormatUint(m.configHash, 10))
	worker.ExtraFiles = m.listenerFiles

	stdout, err := worker.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := worker.StderrPipe()
	if err != nil {
		return err
	}

	tag := "worker:"

	go func() {
		r := bufio.NewReader(stdout)
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					m.config.Logger.Error("reading worker stdout error:", err)
				}
				break
			}
			io.WriteString(os.Stdout, line)
			m.emit(&event{name: workerStdoutEvent, data: strings.TrimRight(line, "\r\n")})
		}
	}()

	go func() {
		r := bufio.NewReader(stderr)
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					m.config.Logger.Error("reading worker stderr error:", err)
				}
				break
			}
			io.WriteString(os.Stdout, line)
			m.emit(&event{name: workerStderrEvent, data: strings.TrimRight(line, "\r\n")})
		}
	}()

	if err := worker.Start(); err != nil {
		return err
	}

	tag = fmt.Sprintf("worker/%d:", worker.Process.Pid)
	pidString := strconv.Itoa(worker.Process.Pid)
	m.startingWorker = worker

	m.emit(&event{name: workerStartEvent, data: pidString})

	if m.config.ReadyTimeout > 0 {
		go func() {
			<-time.After(m.config.ReadyTimeout)
			m.mu.Lock()
			defer m.mu.Unlock()

			if m.startingWorker != worker {
				return
			}

			m.config.Logger.Error(m.colors.Bold(tag), m.colors.Red(fmt.Sprintf(
				"worker did not report ready in [%v] since started, killing it",
				m.config.ReadyTimeout,
			)))
			m.startingWorker.Process.Kill()
			m.startingWorker = nil

			m.emit(&event{name: workerErrorEvent, data: pidString})
		}()
	}

	go func() {
		err := worker.Wait()
		if err == nil {
			m.config.Logger.Info(m.colors.Bold(tag), m.colors.Brown("worker exited"))

		} else {
			if exit, ok := err.(*exec.ExitError); ok {
				if status, ok := exit.Sys().(syscall.WaitStatus); ok && status.ExitStatus() == ConfigHashMismatchExitCode {
					m.restartC <- struct{}{}
				} else {
					m.config.Logger.Error(m.colors.Bold(tag), m.colors.Red(err))
				}
			} else {
				m.config.Logger.Error(m.colors.Bold(tag), m.colors.Red(err))
			}

			m.mu.RLock()
			if worker == m.startingWorker || worker == m.runningWorker {
				m.emit(&event{name: workerErrorEvent, data: pidString})
			}
			m.mu.RUnlock()
		}
	}()

	return nil
}
