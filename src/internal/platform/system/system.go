package system

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"unicode"
	"unicode/utf8"
)

const ShareName = "HachimoConfig"

type State struct {
	Available, Enabled, Running, PasswordSet, CanRestart bool
	Username, ShareName, Error                           string
}
type Credentials struct{ Username, Password string }
type account struct {
	Username string `json:"username"`
}

type Manager struct {
	mu                                    sync.Mutex
	base, configPath, service, passwdPath string
	validate                              Validator
	run                                   func(context.Context, string, []string, []byte) error
	signal                                func() error
	supervised                            bool
}

func New(configPath string, validate Validator) *Manager {
	m := &Manager{base: filepath.Dir(configPath), configPath: configPath,
		service: "/etc/init.d/S98hachimodock-smb", passwdPath: "/etc/passwd",
		validate: validate, supervised: os.Getenv("HACHIMODOCK_SUPERVISED") == "1"}
	m.run = func(ctx context.Context, name string, args []string, stdin []byte) error {
		cmd := exec.CommandContext(ctx, name, args...)
		if stdin != nil {
			cmd.Stdin = bytes.NewReader(stdin)
		}
		// Commands receive passwords only through stdin, and never echo them.
		return cmd.Run()
	}
	m.signal = func() error { return syscall.Kill(os.Getpid(), syscall.SIGTERM) }
	return m
}
func (m *Manager) private(name string) string { return filepath.Join(m.base, "system", "smb", name) }
func (m *Manager) wrapper(name string) string { return filepath.Join(m.base, "samba", name) }
func exists(path string) bool                 { st, err := os.Stat(path); return err == nil && st.Mode().IsRegular() }
func executable(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.Mode().IsRegular() && st.Mode().Perm()&0111 != 0
}
func (m *Manager) readAccount() account {
	var a account
	data, _ := os.ReadFile(m.private("account.json"))
	_ = json.Unmarshal(data, &a)
	return a
}
func (m *Manager) Read(ctx context.Context) State {
	m.mu.Lock()
	defer m.mu.Unlock()
	a := m.readAccount()
	s := State{Username: a.Username, ShareName: ShareName, CanRestart: m.supervised && m.configPath != ""}
	if s.Username == "" {
		s.Username = "dockconfig"
	}
	s.Available = m.configPath != "" && executable(m.wrapper("smbd")) && executable(m.wrapper("smbpasswd")) && executable(m.service)
	s.PasswordSet = a.Username != "" && exists(m.private("smbpasswd"))
	s.Enabled = s.PasswordSet && exists(m.private("enabled"))
	if !s.Available {
		s.Error = "SMB 共享服务未安装"
		return s
	}
	if s.Enabled {
		s.Running = m.run(ctx, m.service, []string{"status"}, nil) == nil
		if !s.Running {
			s.Error = "共享服务未运行，请重新保存账号或开启共享"
		}
	}
	return s
}

var usernamePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

func ValidateCredentials(c Credentials) error {
	if !usernamePattern.MatchString(c.Username) || c.Username == "root" || c.Username == "nobody" {
		return errors.New("用户名需以小写字母开头，使用小写字母、数字、- 或 _，最长 32 位")
	}
	if !utf8.ValidString(c.Password) || len([]rune(c.Password)) < 8 || len(c.Password) > 128 {
		return errors.New("共享密码至少 8 个字符，最多 128 字节")
	}
	for _, r := range c.Password {
		if unicode.IsControl(r) {
			return errors.New("共享密码不能包含控制字符")
		}
	}
	return nil
}
func (m *Manager) smbConfiguration(username string) []byte {
	// All paths are generated locally, never read from remote account fields.
	return []byte(fmt.Sprintf(`[global]
  server role = standalone server
  security = user
  workgroup = WORKGROUP
  netbios name = HACHIMODOCK
  server min protocol = SMB3_00
  server max protocol = SMB3_11
  server signing = mandatory
  smb encrypt = required
  map to guest = Never
  restrict anonymous = 2
  smb ports = 445
  disable netbios = yes
  load printers = no
  printing = bsd
  printcap name = /dev/null
  disable spoolss = yes
  unix extensions = no
  passdb backend = smbpasswd:%s
  private dir = %s
  state directory = %s
  cache directory = /tmp/hachimodock-smb
  lock directory = /tmp/hachimodock-smb
  pid directory = /tmp/hachimodock-smb
  ncalrpc dir = /tmp/hachimodock-smb/ncalrpc
  log file = /var/log/hachimodock-smb.log
  max log size = 128
  logging = file
  log level = 0
  server multi channel support = no
  wide links = no

[%s]
  path = %s
  read only = no
  browseable = yes
  guest ok = no
  valid users = %s
  force user = root
  force group = root
  follow symlinks = no
  wide links = no
  create mask = 0600
  force create mode = 0600
  directory mask = 0700
  force directory mode = 0700
  store dos attributes = no
  ea support = no
`, m.private("smbpasswd"), filepath.Dir(m.private("account.json")), m.private("state"), ShareName, filepath.Join(m.base, "config-share"), username))
}

type savedFile struct {
	data    []byte
	present bool
}

func saveFile(path string) savedFile { b, err := os.ReadFile(path); return savedFile{b, err == nil} }
func restoreFile(path string, saved savedFile) {
	if saved.present {
		_ = atomicWrite(path, saved.data, 0600)
	} else {
		_ = os.Remove(path)
	}
}
func (m *Manager) ConfigureSMB(ctx context.Context, c Credentials) error {
	if err := ValidateCredentials(c); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !executable(m.wrapper("smbpasswd")) || !executable(m.service) {
		return errors.New("共享服务未安装")
	}
	if err := os.MkdirAll(m.private("state"), 0700); err != nil {
		return errors.New("无法创建共享配置目录")
	}
	old := m.readAccount()
	created := false
	if c.Username != old.Username {
		passwd, err := os.ReadFile(m.passwdPath)
		if err != nil {
			return errors.New("无法检查系统账号")
		}
		for _, line := range strings.Split(string(passwd), "\n") {
			if strings.HasPrefix(line, c.Username+":") {
				return errors.New("该用户名已被系统占用，请选择另一个共享用户名")
			}
		}
		if err := m.run(ctx, "adduser", []string{"-D", "-H", "-s", "/bin/false", c.Username}, nil); err != nil {
			return errors.New("无法创建独立共享账号")
		}
		created = true
	}
	names := []string{"smb.conf", "smbpasswd", "account.json", "enabled"}
	saved := make(map[string]savedFile)
	for _, name := range names {
		saved[name] = saveFile(m.private(name))
	}
	complete := false
	defer func() {
		if complete {
			return
		}
		for _, name := range names {
			restoreFile(m.private(name), saved[name])
		}
		clean, cancel := context.WithTimeout(context.Background(), 5e9)
		defer cancel()
		if created {
			_ = m.run(clean, "deluser", []string{c.Username}, nil)
		}
		_ = m.run(clean, m.service, []string{"restart"}, nil)
	}()
	candidate, err := os.CreateTemp(filepath.Dir(m.private("smb.conf")), ".smb-config-*")
	if err != nil {
		return errors.New("无法准备共享账号")
	}
	path := candidate.Name()
	candidate.Close()
	defer os.Remove(path)
	if err = atomicWrite(path, m.smbConfiguration(c.Username), 0600); err != nil {
		return errors.New("无法准备共享配置")
	}
	input := []byte(c.Password + "\n" + c.Password + "\n")
	err = m.run(ctx, m.wrapper("smbpasswd"), []string{"-c", path, "-s", "-a", c.Username}, input)
	clear(input)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("共享账号保存失败")
	}
	if err = os.Chmod(m.private("smbpasswd"), 0600); err != nil {
		return errors.New("无法保护共享账号文件")
	}
	if err = os.Rename(path, m.private("smb.conf")); err != nil {
		return errors.New("无法保存共享配置")
	}
	data, _ := json.Marshal(account{Username: c.Username})
	if err = atomicWrite(m.private("account.json"), data, 0600); err != nil {
		return errors.New("无法保存共享用户名")
	}
	if err = atomicWrite(m.private("enabled"), nil, 0600); err != nil {
		return errors.New("无法开启共享")
	}
	if err = m.run(ctx, m.service, []string{"restart"}, nil); err != nil {
		return errors.New("共享服务启动失败，原设置已恢复")
	}
	if err = m.run(ctx, m.service, []string{"status"}, nil); err != nil {
		return errors.New("共享服务未就绪，原设置已恢复")
	}
	complete = true
	if old.Username != "" && old.Username != c.Username {
		_ = m.run(ctx, m.wrapper("smbpasswd"), []string{"-c", m.private("smb.conf"), "-x", old.Username}, nil)
		_ = m.run(ctx, "deluser", []string{old.Username}, nil)
	}
	return nil
}

func (m *Manager) SetSMB(ctx context.Context, enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !executable(m.service) {
		return errors.New("共享服务未安装")
	}
	if enabled {
		if m.readAccount().Username == "" || !exists(m.private("smbpasswd")) {
			return errors.New("请先设置共享用户名和密码")
		}
		if err := atomicWrite(m.private("enabled"), nil, 0600); err != nil {
			return err
		}
		if err := m.run(ctx, m.service, []string{"restart"}, nil); err != nil {
			_ = os.Remove(m.private("enabled"))
			return errors.New("共享服务启动失败")
		}
		return nil
	}
	if err := m.run(ctx, m.service, []string{"stop"}, nil); err != nil {
		return errors.New("共享服务停止失败")
	}
	if err := os.Remove(m.private("enabled")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (m *Manager) Restart(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.supervised {
		return errors.New("当前未由开机服务管理，不能自动重启应用")
	}
	if err := ApplyConfiguration(ctx, m.configPath, m.validate); err != nil {
		return fmt.Errorf("配置未生效：%w", err)
	}
	if err := m.signal(); err != nil {
		return errors.New("配置已应用，但无法请求重启")
	}
	return nil
}
