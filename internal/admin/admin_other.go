//go:build !windows

package admin

func IsAdmin() bool { return true }

func RelaunchAsAdmin() bool { return false }

func RequireAdmin(skip bool) bool { return true }
