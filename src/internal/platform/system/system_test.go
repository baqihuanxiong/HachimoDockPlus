package system

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func managerFixture(t *testing.T) *Manager {
	t.Helper()
	path := configurationFixture(t)
	if err := PrepareConfiguration(path); err != nil {
		t.Fatal(err)
	}
	m := New(path, validateFixture)
	m.service = filepath.Join(m.base, "service")
	m.passwdPath = filepath.Join(m.base, "passwd")
	os.WriteFile(m.passwdPath, []byte("root:x:0:0:root:/root:/bin/sh\n"), 0600)
	os.MkdirAll(filepath.Join(m.base, "samba"), 0700)
	for _, path := range []string{m.service, m.wrapper("smbd"), m.wrapper("smbpasswd")} {
		os.WriteFile(path, []byte("#!/bin/sh\n"), 0700)
	}
	return m
}
func TestSMBCredentialsNeverEnterArgumentsSnapshotOrPlaintextStorage(t *testing.T) {
	m := managerFixture(t)
	password := "unique-password-never-log"
	calls := 0
	m.run = func(ctx context.Context, name string, args []string, input []byte) error {
		calls++
		for _, arg := range args {
			if strings.Contains(arg, password) {
				t.Fatal("password in argv")
			}
		}
		if name == m.wrapper("smbpasswd") && len(input) > 0 {
			if string(input) != password+"\n"+password+"\n" {
				t.Fatal("password not sent through stdin")
			}
			return os.WriteFile(m.private("smbpasswd"), []byte("test-nt-hash-only"), 0600)
		}
		return nil
	}
	if err := m.ConfigureSMB(context.Background(), Credentials{"docktest", password}); err != nil {
		t.Fatal(err)
	}
	if calls < 3 {
		t.Fatal("account/service operations missing")
	}
	s := m.Read(context.Background())
	if s.Username != "docktest" || !s.Enabled || !s.PasswordSet || !s.Running {
		t.Fatal(s)
	}
	filepath.WalkDir(filepath.Join(m.base, "system"), func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			b, _ := os.ReadFile(path)
			if bytes.Contains(b, []byte(password)) {
				t.Errorf("password stored in %s", path)
			}
		}
		return err
	})
}
func TestAccountFailureRestoresPreviousSettings(t *testing.T) {
	m := managerFixture(t)
	m.run = func(ctx context.Context, name string, args []string, input []byte) error {
		if name == m.wrapper("smbpasswd") && len(input) > 0 {
			return os.WriteFile(m.private("smbpasswd"), []byte("old-hash"), 0600)
		}
		return nil
	}
	if err := m.ConfigureSMB(context.Background(), Credentials{"dockold", "original-password"}); err != nil {
		t.Fatal(err)
	}
	oldConfig := readTest(t, m.private("smb.conf"))
	m.run = func(ctx context.Context, name string, args []string, input []byte) error {
		if name == m.wrapper("smbpasswd") && len(input) > 0 {
			return os.WriteFile(m.private("smbpasswd"), []byte("new-hash"), 0600)
		}
		if name == m.service && len(args) > 0 && args[0] == "restart" {
			return errors.New("failed")
		}
		return nil
	}
	if err := m.ConfigureSMB(context.Background(), Credentials{"docknew", "replacement-password"}); err == nil {
		t.Fatal("failed service reported success")
	}
	if readTest(t, m.private("smbpasswd")) != "old-hash" || readTest(t, m.private("smb.conf")) != oldConfig || m.readAccount().Username != "dockold" {
		t.Fatal("previous account was not restored")
	}
}
func TestSMBRejectsReservedExistingAndInvalidAccounts(t *testing.T) {
	for _, c := range []Credentials{{"root", "password123"}, {"../bad", "password123"}, {"valid", "short"}, {"valid", "pass\nword123"}} {
		if ValidateCredentials(c) == nil {
			t.Fatal("invalid credentials accepted", c.Username)
		}
	}
	m := managerFixture(t)
	os.WriteFile(m.passwdPath, []byte("existing:x:10:10::/:/bin/sh\n"), 0600)
	m.run = func(context.Context, string, []string, []byte) error {
		t.Fatal("existing account was modified")
		return nil
	}
	if err := m.ConfigureSMB(context.Background(), Credentials{"existing", "password123"}); err == nil {
		t.Fatal("system account accepted")
	}
}
func TestRestartValidatesBeforeSignalingAndRequiresSupervisor(t *testing.T) {
	m := managerFixture(t)
	signaled := false
	m.signal = func() error { signaled = true; return nil }
	m.supervised = false
	if err := m.Restart(context.Background()); err == nil || signaled {
		t.Fatal("unsupervised process restarted")
	}
	m.supervised = true
	os.WriteFile(filepath.Join(m.base, "config-share", "config.json"), []byte("{"), 0600)
	if err := m.Restart(context.Background()); err == nil || signaled {
		t.Fatal("invalid configuration triggered restart")
	}
	os.WriteFile(filepath.Join(m.base, "config-share", "config.json"), []byte(`{"model":"new"}`), 0600)
	if err := m.Restart(context.Background()); err != nil || !signaled {
		t.Fatal("valid restart did not signal", err)
	}
}
