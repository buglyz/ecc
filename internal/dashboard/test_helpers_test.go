package dashboard

import "github.com/buglyz/ecc/internal/paths"

func structPaths() paths.Paths {
	return paths.Paths{}
}

type fakeStartupController struct {
	enabled bool
	addErr  error
	rmErr   error
}

func (f *fakeStartupController) IsEnabled(string) bool {
	return f.enabled
}

func (f *fakeStartupController) Add(string, string) error {
	if f.addErr != nil {
		return f.addErr
	}
	f.enabled = true
	return nil
}

func (f *fakeStartupController) Remove(string) error {
	if f.rmErr != nil {
		return f.rmErr
	}
	f.enabled = false
	return nil
}
