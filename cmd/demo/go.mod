module github.com/drummonds/gobank/cmd/demo

go 1.25.3

require (
	github.com/drummonds/lofigui v0.17.5
	github.com/flosch/pongo2/v6 v6.0.0
)

require github.com/russross/blackfriday/v2 v2.1.0 // indirect

replace github.com/drummonds/lofigui => ../../../../minor/lofigui
