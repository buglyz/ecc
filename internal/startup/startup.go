package startup

import (
	"os/exec"
	"os/user"

	"github.com/buglyz/ecc/internal/process"
)

const Identifier = "风扇控制"

func IsEnabled(identifier string) bool {
	cmd := exec.Command("schtasks.exe", "/Query", "/TN", taskName(identifier))
	cmd.SysProcAttr = process.HiddenSysProcAttr()
	return cmd.Run() == nil
}

func Add(targetPath, identifier string) error {
	u, err := user.Current()
	if err != nil {
		return err
	}

	cmd := exec.Command("schtasks.exe", "/Create",
		"/TN", taskName(identifier),
		"/TR", `"`+targetPath+`"`,
		"/SC", "ONLOGON",
		"/RL", "HIGHEST",
		"/RU", u.Username,
		"/F",
	)
	cmd.SysProcAttr = process.HiddenSysProcAttr()
	return cmd.Run()
}

func Remove(identifier string) error {
	if !IsEnabled(identifier) {
		return nil
	}
	cmd := exec.Command("schtasks.exe", "/Delete", "/TN", taskName(identifier), "/F")
	cmd.SysProcAttr = process.HiddenSysProcAttr()
	return cmd.Run()
}

func taskName(identifier string) string {
	return identifier
}
