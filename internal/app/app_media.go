package app

import (
	"encoding/base64"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/myrheimb/pageroom/internal/media"
)

func (a *App) library() (*media.Library, error) {
	if _, err := a.requireSite(); err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.media, nil
}

func (a *App) ListMedia() ([]media.Item, error) {
	l, err := a.library()
	if err != nil {
		return nil, err
	}
	return l.List()
}

func (a *App) PickImages() ([]string, error) {
	return application.Get().Dialog.OpenFile().
		SetTitle("Choose images").
		AddFilter("Images", "*.jpg;*.jpeg;*.png;*.gif;*.webp;*.svg;*.JPG;*.JPEG;*.PNG").
		PromptForMultipleSelection()
}

// ImportFailure is reported per file so one bad image doesn't block the rest.
type ImportFailure struct {
	Name    string `json:"name"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}

type ImportOutcome struct {
	Imported []media.ImportResult `json:"imported"`
	Failed   []ImportFailure      `json:"failed"`
}

// ImportMedia imports files from disk (picker or drag and drop). page is
// the page being edited ("" for the shared library).
func (a *App) ImportMedia(paths []string, page string) (*ImportOutcome, error) {
	l, err := a.library()
	if err != nil {
		return nil, err
	}
	out := &ImportOutcome{}
	for _, p := range paths {
		name := filepath.Base(p)
		st, err := os.Stat(p)
		if err != nil || st.IsDir() {
			out.Failed = append(out.Failed, ImportFailure{Name: name, Code: "not-a-file", Message: "This isn't a file."})
			continue
		}
		if st.Size() > media.MaxUpload {
			out.Failed = append(out.Failed, ImportFailure{Name: name, Code: "file-too-large", Message: "This file is too large (max 60 MB)."})
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			out.Failed = append(out.Failed, ImportFailure{Name: name, Message: err.Error()})
			continue
		}
		res, err := l.Import(data, name, page)
		if err != nil {
			out.Failed = append(out.Failed, ImportFailure{Name: name, Message: err.Error()})
			continue
		}
		out.Imported = append(out.Imported, *res)
	}
	return out, nil
}

func (a *App) ImportMediaData(name, dataBase64, page string) (*media.ImportResult, error) {
	l, err := a.library()
	if err != nil {
		return nil, err
	}
	data, err := base64.StdEncoding.DecodeString(dataBase64)
	if err != nil {
		return nil, &AppError{Code: "image-unreadable", Message: "That image couldn't be read."}
	}
	if name == "" {
		name = "pasted-image"
	}
	res, err := l.Import(data, name, page)
	if err != nil {
		return nil, &AppError{Code: "invalid", Message: err.Error()}
	}
	return res, nil
}

func (a *App) MediaUsage(path string) ([]string, error) {
	l, err := a.library()
	if err != nil {
		return nil, err
	}
	return l.Usage(path), nil
}

// DeleteMedia moves an image to the trash. Images still used on pages are
// refused unless force is set (after the UI has warned the user).
func (a *App) DeleteMedia(path string, force bool) error {
	l, err := a.library()
	if err != nil {
		return err
	}
	if !force {
		if used := l.Usage(path); len(used) > 0 {
			return &AppError{Code: "in-use", Message: "This image is used on a page.", Path: used[0]}
		}
	}
	return l.Delete(path)
}
