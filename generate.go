// +build generate

package main

import (
	"bytes"
	"io/ioutil"
	"log"
	"os"
	"text/template"
)

func main() {
	t := template.Must(template.New("").Parse(tmpl))

	b, err := ioutil.ReadFile(os.Args[1])
	if err != nil {
		log.Fatalln(err)
	}
	b = bytes.ReplaceAll(b, []byte("`"), []byte("`+\"`\"+`"))

	var buf bytes.Buffer
	err = t.Execute(&buf, map[string]string{"TemplateStr": string(b)})
	if err != nil {
		log.Fatalln(err)
	}
	err = ioutil.WriteFile("template.go", buf.Bytes(), 0644)
	if err != nil {
		log.Fatalln(err)
	}
}

var tmpl = `// Code generated by generate.go DO NOT EDIT.
package main

const (
        tmplStr = ` + "`" + `{{ .TemplateStr }}` + "`" + `
)
`
