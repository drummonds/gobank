//go:build !(js && wasm)

package main

import "strings"

func init() {
	phonePreviewFunc = renderPhonePreview
}

// phoneFrameCSS contains scoped CSS for the inline phone preview.
const phoneFrameCSS = `
.phone-preview .phone-frame {
  width: 300px; height: 600px;
  border-radius: 36px;
  border: 5px solid #1c1c1e;
  background: #fff;
  box-shadow: 0 10px 30px rgba(0,0,0,0.2);
  overflow: hidden;
  position: relative;
  display: flex; flex-direction: column;
}
.phone-preview .phone-notch {
  width: 120px; height: 22px;
  background: #1c1c1e;
  border-radius: 0 0 14px 14px;
  margin: 0 auto;
  position: relative; z-index: 10;
}
.phone-preview .phone-header {
  background: #00947e; color: #fff;
  padding: 6px 12px 8px; text-align: center;
}
.phone-preview .phone-header .title { color: #fff; margin: 0; font-size: 0.9rem; }
.phone-preview .phone-header .subtitle { color: rgba(255,255,255,0.8); margin: 0; font-size: 0.7rem; }
.phone-preview .phone-body {
  flex: 1; overflow-y: auto;
  padding: 8px 12px;
  -webkit-overflow-scrolling: touch;
}
.phone-preview .phone-nav {
  background: #fafafa; border-top: 1px solid #eee;
  display: flex; padding: 6px 0 8px;
}
.phone-preview .phone-nav a {
  flex: 1; text-align: center; color: #7a7a7a;
  font-size: 0.6rem; text-decoration: none;
}
.phone-preview .phone-nav a.is-active { color: #00947e; font-weight: bold; }
.phone-preview .phone-nav a span { display: block; font-size: 1rem; }
`

func renderPhonePreview(ds *DemoState, custID string, accountIdx int) string {
	var inner string
	if accountIdx < 0 {
		inner = ds.buildAppBalanceHTML(custID)
	} else {
		inner = ds.buildAppProductHTML(custID, accountIdx, 1)
	}

	var s strings.Builder
	s.WriteString(`<div class="box phone-preview">`)
	s.WriteString(`<style>`)
	s.WriteString(phoneFrameCSS)
	s.WriteString(`</style>`)
	s.WriteString(`<p class="title is-6 mb-3">Customer App View</p>`)
	s.WriteString(`<div class="phone-frame">`)
	s.WriteString(`<div class="phone-notch"></div>`)
	s.WriteString(inner)
	s.WriteString(`</div></div>`)
	return s.String()
}
