package site

import (
	_ "embed"
	"fmt"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/myrheimb/pageroom/internal/fsutil"
)

// ContactFormShortcode is what a page contains to show the contact form.
const ContactFormShortcode = "{{< contact-form >}}"

const contactFormFile = "layouts/shortcodes/contact-form.html"

//go:embed contact-form.html
var contactFormSource string

var contactFormTemplate = template.Must(template.New("contact-form").Delims("[[", "]]").Parse(contactFormSource))

type contactFormLabels struct {
	Name, Email, Message, Send, PreviewOnly, EmailUs, SetUp string
}

const emailLink = `<a href="mailto:{{ $email }}">{{ $email }}</a>`

// The labels are written into the site in its own language, because the
// Hugo functions that tell the language differ between Hugo versions.
var contactFormLanguages = map[string]contactFormLabels{
	"en": {
		Name: "Name", Email: "Email", Message: "Message", Send: "Send",
		PreviewOnly: "Sending is switched off in the preview.",
		EmailUs:     fmt.Sprintf("Email us at %s.", emailLink),
		SetUp:       "This contact form appears once you add a form address or an email in Pageroom, under Settings → Site. Only the preview shows this note.",
	},
	"ja": {
		Name: "お名前", Email: "メールアドレス", Message: "お問い合わせ内容", Send: "送信",
		PreviewOnly: "プレビューでは送信できません。",
		EmailUs:     fmt.Sprintf("お問い合わせは %s までメールでお送りください。", emailLink),
		SetUp:       "Pageroomの「設定」→「サイト」で「お問い合わせフォームの送信先」か「お問い合わせ用メールアドレス」を入力すると、ここにお問い合わせフォームが表示されます。この案内はプレビューにだけ表示されます。",
	},
}

// AddContactForm puts the contact form shortcode into the site, unless the
// site already has its own.
func (s *Site) AddContactForm() error {
	full := filepath.Join(s.Root, filepath.FromSlash(contactFormFile))
	if fsutil.Exists(full) {
		return nil
	}
	labels := contactFormLanguages["en"]
	if strings.HasPrefix(strings.ToLower(s.languageValue()), "ja") {
		labels = contactFormLanguages["ja"]
	}
	var out strings.Builder
	if err := contactFormTemplate.Execute(&out, labels); err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(full, []byte(out.String()), 0o644)
}
