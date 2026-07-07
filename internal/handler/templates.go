package handler

import "html/template"

func parseTemplates(names ...string) (*template.Template, error) {
	tmpl := template.New("")
	for _, name := range names {
		data, err := templateFS.ReadFile("template/" + name)
		if err != nil {
			return nil, err
		}
		tmpl, err = tmpl.Parse(string(data))
		if err != nil {
			return nil, err
		}
	}
	return tmpl, nil
}
