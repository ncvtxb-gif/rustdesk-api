package templates

import (
	"bytes"
	"html/template"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestOAuthSuccessPageCommunicatesCompletedFeishuLogin(t *testing.T) {
	tmpl, err := template.ParseFiles("oauth_success.html")
	if err != nil {
		t.Fatalf("parse success template: %v", err)
	}

	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, map[string]string{"message": "OauthSuccess"}); err != nil {
		t.Fatalf("render success template: %v", err)
	}

	doc, err := html.Parse(&rendered)
	if err != nil {
		t.Fatalf("parse rendered HTML: %v", err)
	}

	visibleText := textContent(doc)
	for _, want := range []string{
		"飞书授权成功",
		"身份验证已完成，请返回 RustDesk 客户端继续使用。",
		"关闭页面",
		"已安全完成身份验证",
	} {
		if !strings.Contains(visibleText, want) {
			t.Errorf("rendered page is missing %q", want)
		}
	}
}

func textContent(node *html.Node) string {
	if node.Type == html.TextNode {
		return node.Data
	}

	var text strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		text.WriteString(textContent(child))
	}
	return text.String()
}
