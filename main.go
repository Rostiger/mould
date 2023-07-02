package main

import (
	"fmt"
	"bytes"
	"strings"
	"html/template"
	"regexp"
	"path/filepath"
	"flag"
	"bufio"
	. "github.com/dave/jennifer/jen"
	"os"
)

/*
examples of structs that would be generated by this code, depending on the input format:

type FormContent struct {
	Title string
	Password string
	HeaderImage string
	Description string
}

type FormAnswer struct {
	Name string `json:"name"`
	Address string `json:"address"`
	StickerSheetAmount int `json:"sticker-sheet-amount"
	AccessToken string `json:"access-token"`
}


input format expected (whitespace around equals sign is irrelevant):

form-title          = Merveilles Stickers
form-desc           = Hey mervs, welcome to this form! Fill in the inputs below and then press submit!
form-password       = hihi-stickertown
input[Name]         = First and last name
textarea[Address]   = your postal address
number[Sticker sheet amount]#amount                 = min=1, max=5, value=1
input[The rabbit boat but backwards]#access-token   = you know it.
radio[Size]                                         = Small, Medium, Large
*/

func jsonTag (value string) map[string]string {
	return map[string]string{"json":value}
}

type genValue struct {
	element string
	title string
	value string
	key string
	required bool
	options map[string]string
}

type Theme struct {
	background, title, body string
}

type StyleData struct {
	Background, TitleColor, Body template.HTML
}

type TemplateData struct {
	Content template.HTML
	Title string
}

var stylesheetTemplate = `<style>
		html {
			background: {{ .Background }};
			color: {{ .Body }};
			padding-left: 2rem;
			padding-right: 2rem;
			padding-top: 1rem;
		}
		h1 {
			color: {{ .TitleColor }};
		}
		* {
			padding: 0;
			margin-bottom: 0.5rem;
		}
		div {
			display: grid;
			max-width: 600px;
			align-items: center;
		}
</style>
`

func parseFormat(format string) []genValue {
	pattern := regexp.MustCompile(`(form-\w+)|([!]?)(\S*)(\[.*\])([#]\S+)?`)
	scanner := bufio.NewScanner(strings.NewReader(format))
	var genList []genValue
	for scanner.Scan() {
		line := scanner.Text()
		splitterIndex := strings.Index(line, "=")
		left := strings.TrimSpace(line[0:splitterIndex])

		var v genValue 
		v.value = strings.TrimSpace(line[splitterIndex+1:])
		matches := pattern.FindStringSubmatch(left)
		if len(matches) > 2 && matches[2] == "!" {
			v.required = true
		}
		if matches[1] != "" {
			v.element = strings.TrimSpace(matches[1])
		} else if matches[3] != "" {
			v.element = strings.TrimSpace(matches[3])
		}
		if len(matches) > 4 && matches[4] != "" {
			// get everything except [thing] brackets
			v.title = matches[4][1:len(matches[4])-1]
		}
		if len(matches) > 5 && matches[5] != "" {
			// remove initial #
			v.key = strings.TrimSpace(matches[5][1:])
		}
		genList = append(genList, v)
	}
	return genList
}

var htmlTemplate = `<!DOCTYPE html>
<html>
	<head>
		<title>{{ .Title }}</title>
		%SENTINEL%
	</head>
	<body>
	{{ .Content }}
	</body>
</html>`

var responseTemplate = `<!DOCTYPE html>
<html>
    <head>
    <title>Form submitted</title>
		%SENTINEL%
    <body>
			<h1>Response successful</h1>
			<p>Your response: </p>
			<pre>
			<code>
{{ .Data }}
			</code>
			</pre>
			<p><b>Bookmark this page</b> as a receipt or if you want to review what you responded some time in the future</p>
	</body>
</html>`

func formatKeyAndTitle(v genValue) (string, string) {
	key := strings.ToLower(v.title)
	title := strings.ReplaceAll(strings.Title(v.title), " ", "")
	if len(v.key) > 0 {
		key = v.key
		title = strings.ReplaceAll(strings.Title(strings.ReplaceAll(v.key, "-", " ")), " ", "")
	}
	return key, title
}

const formPackageName = "myform"
func main() {
	var htmlList []string
	var theme Theme
	var setPassword string
	setUser := "mouldy" // default user is "mouldy". only used if password is set, and can be changed with `form-user`
	var pageTitle string
	var formatFp string
	var stylesheetFp string
	flag.StringVar(&stylesheetFp, "stylesheet", "", "a single css file containing styles that will be applied to the form (fully replaces mould's default styling)")
	flag.StringVar(&formatFp, "input", "", "a file containing the form format to generate a form server using")
	flag.Parse()
	if formatFp == "" {
		fmt.Println("must pass --input <file containing form format>")
		os.Exit(0)
	}
	b, err := os.ReadFile(formatFp)
	if err != nil {
		fmt.Println("issue when reading format file", err)
	}
	format := string(b)

	values := parseFormat(format)

	f := NewFile(formPackageName)
	var contentBits []Code
	var answer []Code
	var resParse []Code
	for _, input := range values {
		switch input.element {
		case "form-title":
			contentBits = append(contentBits, Id("Title").String())
			htmlList = append(htmlList, fmt.Sprintf(`<h1>%s</h1>`, input.value))
			pageTitle = input.value
		case "form-desc":
			contentBits = append(contentBits, Id("Description").String())
			htmlList = append(htmlList, fmt.Sprintf(`<p>%s</p>`, input.value))
		case "form-image":
			contentBits = append(contentBits, Id("Image").String())
			htmlList = append(htmlList, fmt.Sprintf(`<img src="%s">`, input.value))
		case "form-password":
			setPassword = input.value
			// information used for basic auth, limiting access to the form
			contentBits = append(contentBits, Id("Password").String())
		case "form-user":
			setUser = input.value
			// information used for basic auth, limiting access to the form
			contentBits = append(contentBits, Id("User").String())
		case "form-bg":
			theme.background = input.value
		case "form-titlecolor":
			theme.title = input.value
		case "form-fg":
			theme.body = input.value
		}
	}

	htmlList = append(htmlList, `<form action="/" method="post">`)
	for _, input := range values {
			var required string 
			if input.required {
				required = `required`
			}
		switch input.element {
		case "textarea":
			key, title := formatKeyAndTitle(input)
			htmlList = append(htmlList, "<div>")
			htmlList = append(htmlList, fmt.Sprintf(`<label for="%s">%s</label>`, key, title))
			el := fmt.Sprintf(`<textarea %s placeholder="%s" name="%s"></textarea>`, required, input.value, key)
			htmlList = append(htmlList, el)
			htmlList = append(htmlList, "</div>")
			answer = append(answer, Id(title).String().Tag(jsonTag(key)))
			resParse = append(resParse, Id("answer").Dot(title).Op("=").Id("req").Dot("PostFormValue").Call(Lit(key)))
		case "input":
			key, title := formatKeyAndTitle(input)
			htmlList = append(htmlList, "<div>")
			htmlList = append(htmlList, fmt.Sprintf(`<label for="%s">%s</label>`, key, input.title))
			el := fmt.Sprintf(`<input type="text" %s placeholder="%s" name="%s"/>`, required, input.value, key)
			htmlList = append(htmlList, el)
			htmlList = append(htmlList, "</div>")
			answer = append(answer, Id(title).String().Tag(jsonTag(key)))
			resParse = append(resParse, Id("answer").Dot(title).Op("=").Id("req").Dot("PostFormValue").Call(Lit(key)))
		case "hidden":
			key, title := formatKeyAndTitle(input)
			htmlList = append(htmlList, "<div>")
			el := fmt.Sprintf(`<input type="hidden" %s value="%s" name="%s"/>`, required, input.value, key)
			htmlList = append(htmlList, el)
			htmlList = append(htmlList, "</div>")
			answer = append(answer, Id(title).String().Tag(jsonTag(key)))
			resParse = append(resParse, Id("answer").Dot(title).Op("=").Id("req").Dot("PostFormValue").Call(Lit(key)))
		case "form-paragraph":
			htmlList = append(htmlList, fmt.Sprintf(`<p>%s</p>`, input.value))
		case "email":
			key, title := formatKeyAndTitle(input)
			htmlList = append(htmlList, "<div>")
			htmlList = append(htmlList, fmt.Sprintf(`<label for="%s">%s</label>`, key, input.title))
			el := fmt.Sprintf(`<input type="email" %s placeholder="email@provider.tld" pattern="%s", name="%s"/>`, required, input.value, key)
			htmlList = append(htmlList, el)
			htmlList = append(htmlList, "</div>")
			answer = append(answer, Id(title).String().Tag(jsonTag(key)))
			resParse = append(resParse, Id("answer").Dot(title).Op("=").Id("req").Dot("PostFormValue").Call(Lit(key)))
		case "number":
			optionsList := strings.Split(input.value, ",")
			var options string
			htmlList = append(htmlList, "<div>")
			for _, optionPair := range optionsList {
				optionPair = strings.TrimSpace(optionPair)
				parts := strings.Split(optionPair, "=")
				options += fmt.Sprintf(`%s="%s" `,parts[0], parts[1])
			}
			key, title := formatKeyAndTitle(input)
			htmlList = append(htmlList, fmt.Sprintf(`<label for="%s">%s</label>`, key, title))
			el := fmt.Sprintf(`<input type="number" %s %s name="%s"/>`, required, options, key)
			htmlList = append(htmlList, el)
			htmlList = append(htmlList, "</div>")
			answer = append(answer, Id(title).String().Tag(jsonTag(key)))
			resParse = append(resParse, Id("answer").Dot(title).Op("=").Id("req").Dot("PostFormValue").Call(Lit(key)))
		case "range":
			optionsList := strings.Split(input.value, ",")
			var options string
			htmlList = append(htmlList, "<div>")
			for _, optionPair := range optionsList {
				optionPair = strings.TrimSpace(optionPair)
				parts := strings.Split(optionPair, "=")
				options += fmt.Sprintf(`%s="%s" `,parts[0], parts[1])
			}
			key, title := formatKeyAndTitle(input)
			htmlList = append(htmlList, fmt.Sprintf(`<label for="%s">%s</label>`, key, title))
			el := fmt.Sprintf(`<input type="range" %s %s name="%s"/>`, required, options, key)
			htmlList = append(htmlList, el)
			htmlList = append(htmlList, "</div>")
			answer = append(answer, Id(title).String().Tag(jsonTag(key)))
			resParse = append(resParse, Id("answer").Dot(title).Op("=").Id("req").Dot("PostFormValue").Call(Lit(key)))
		case "radio":
			options := strings.Split(input.value, ",")
			key, title := formatKeyAndTitle(input)

			htmlList = append(htmlList, "<div>")
			htmlList = append(htmlList, fmt.Sprintf(`<span>%s</span>`, input.title))
			for i, val := range options {
				options[i] = strings.TrimSpace(val)
				radioValue := strings.ToLower(options[i])
				radioId := fmt.Sprintf(`%s-option-%s`, key, radioValue)
				htmlList = append(htmlList, "<span>")
				el := fmt.Sprintf(`<input type="radio" id="%s" value="%s" name="%s"/>`, radioId, radioValue, key)
				htmlList = append(htmlList, el)
				htmlList = append(htmlList, fmt.Sprintf(`<label for="%s">%s</label>`, radioId, options[i]))
				htmlList = append(htmlList, "</span>")

			}
			htmlList = append(htmlList, "</div>")
			answer = append(answer, Id(title).String().Tag(jsonTag(key)))
			resParse = append(resParse, Id("answer").Dot(title).Op("=").Id("req").Dot("PostFormValue").Call(Lit(key)))
		}
	}

	htmlList = append(htmlList, `<div><button type="submit">Submit</button></div>`)
	htmlList = append(htmlList, "</form>")

	// set BasicPassword const
	f.Const().Id("BasicPassword").Op("=").Lit(setPassword)
	f.Const().Id("BasicUser").Op("=").Lit(setUser)
	// generate FormContent struct
	f.Type().Id("FormContent").Struct(contentBits...)
	// generate FormAnswer struct
	f.Type().Id("FormAnswer").Struct(answer...)

	// generate FormAnswer.ParsePost() 
	f.Func().Params(
		Id("answer").Id("*FormAnswer"),
	).Id("ParsePost").Params(
		Id("req").Op("*").Qual("net/http", "Request"),
	).Block(resParse...)

	// generate ResponderData struct
	f.Type().Id("ResponderData").Struct(Id("Data").String())

	fmt.Printf("%#v", f)

	// make sure the package folder will exist
	err = os.MkdirAll(formPackageName, 0777)
	if err != nil {
		fmt.Println("err mkdirall", err)
	}
	// write the generated form model to disk
	generatedCode := fmt.Sprintf("%#v", f)
	genCodeErr := os.WriteFile(filepath.Join(formPackageName, "generated-form-model.go"), []byte(generatedCode), 0777)
	if genCodeErr != nil {
		fmt.Println(genCodeErr)
	}
	var data TemplateData
	data.Title = pageTitle
	data.Content = template.HTML(strings.Join(htmlList, "\n"))

	var styleData StyleData
	if theme.background != "" {
		styleData.Background = template.HTML(theme.background)
	}
	if theme.body != "" {
		styleData.Body = template.HTML(theme.body)
	}
	if theme.title != "" {
		styleData.TitleColor = template.HTML(theme.title)
	}

	// stylesheet was passed with --stylesheet command: try to read it and then 
	// *fully* replace the contents of stylesheetTemplate with the passed in style
	if stylesheetFp != "" {
		b, err := os.ReadFile(stylesheetFp)
		if err == nil {
			stylesheetTemplate = fmt.Sprintf("<style>%s</style>", string(b))
		} else {
			fmt.Println("err reading stylesheet", err)
		}
	}

	// render the stylesheet 
	t := template.Must(template.New("").Parse(stylesheetTemplate))
	var buf bytes.Buffer
	t.Execute(&buf, styleData)

	// insert the stylesheet into the head of the document
	htmlTemplate = strings.ReplaceAll(htmlTemplate, "%SENTINEL%", buf.String())
	responseTemplate = strings.ReplaceAll(responseTemplate, "%SENTINEL%", buf.String())

	// write the page htmlList
	t = template.Must(template.New("").Parse(htmlTemplate))
	t.Execute(&buf, data)
	indexWriteErr := os.WriteFile("index-template.html", buf.Bytes(), 0777)
	if indexWriteErr != nil {
		fmt.Println(indexWriteErr)
	}
	indexWriteErr = os.WriteFile("response-template.html", []byte(responseTemplate), 0777)
	if indexWriteErr != nil {
		fmt.Println(indexWriteErr)
	}
}
