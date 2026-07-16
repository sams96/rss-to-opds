package epub

import (
	"encoding/xml"
	"fmt"
	"io"
)

const (
	xhtmlDoctype = `<!DOCTYPE html>
`
	xhtmlLinkRel  = "stylesheet"
	xhtmlTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml">
  <head>
    <title dir="auto"></title>
  </head>
  <body dir="auto"></body>
</html>
`
	mediaTypeCSS = "text/css"
)

// xhtml implements an XHTML document
type xhtml struct {
	xml *xhtmlRoot
}

// This holds the actual XHTML content
type xhtmlRoot struct {
	XMLName   xml.Name      `xml:"http://www.w3.org/1999/xhtml html"`
	XmlnsEpub string        `xml:"xmlns:epub,attr,omitempty"`
	Head      xhtmlHead     `xml:"head"`
	Body      xhtmlInnerxml `xml:"body"`
}

type xhtmlHead struct {
	Title xhtmlTitle `xml:"title"`
	Link  *xhtmlLink
}

type xhtmlTitle struct {
	XMLName xml.Name `xml:"title,omitempty"`
	Dir     string   `xml:"dir,attr,omitempty"`
	Value   string   `xml:",chardata"`
}

// The <link> element, used to link to stylesheets
// Ex: <link rel="stylesheet" type="text/css" href="../css/epub.css" />
type xhtmlLink struct {
	XMLName xml.Name `xml:"link,omitempty"`
	Rel     string   `xml:"rel,attr,omitempty"`
	Type    string   `xml:"type,attr,omitempty"`
	Href    string   `xml:"href,attr,omitempty"`
}

// This holds the content of the XHTML document between the <body> tags. It is
// implemented as a string because we don't know what it will contain and we
// leave it up to the user of the package to validate the content
type xhtmlInnerxml struct {
	XML io.Reader
	Dir string
}

// Constructor for xhtml
func newXhtml(body io.Reader) *xhtml {
	return &xhtml{
		xml: &xhtmlRoot{
			Body: xhtmlInnerxml{XML: body, Dir: "auto"},
		},
	}
}

func (x *xhtml) setCSS(path string) {
	x.xml.Head.Link = &xhtmlLink{
		Rel:  xhtmlLinkRel,
		Type: mediaTypeCSS,
		Href: path,
	}
}

func (x *xhtml) setTitle(title string) {
	x.xml.Head.Title = xhtmlTitle{
		Dir:   "auto",
		Value: title,
	}
}

func (x *xhtml) setXmlnsEpub(xmlns string) {
	x.xml.XmlnsEpub = xmlns
}

func (x *xhtml) Title() string {
	return x.xml.Head.Title.Value
}

func (x *xhtml) write(w io.Writer) error {
	if _, err := w.Write([]byte(xml.Header + xhtmlDoctype)); err != nil {
		return err
	}

	xmlnsAttr := ""
	if x.xml.XmlnsEpub != "" {
		xmlnsAttr = fmt.Sprintf(` xmlns:epub="%s"`, x.xml.XmlnsEpub)
	}
	if _, err := fmt.Fprintf(w, `<html xmlns="http://www.w3.org/1999/xhtml"%s>`, xmlnsAttr); err != nil {
		return err
	}

	if _, err := fmt.Fprintf(w, `<head><title dir="%s">%s</title>`, x.xml.Head.Title.Dir, x.xml.Head.Title.Value); err != nil {
		return err
	}
	if x.xml.Head.Link != nil {
		if _, err := fmt.Fprintf(w, `<link rel="%s" type="%s" href="%s" />`, x.xml.Head.Link.Rel, x.xml.Head.Link.Type, x.xml.Head.Link.Href); err != nil {
			return err
		}
	}
	if _, err := w.Write([]byte("</head>")); err != nil {
		return err
	}

	if _, err := fmt.Fprintf(w, `<body dir="%s">`+"\n", x.xml.Body.Dir); err != nil {
		return err
	}

	if x.xml.Body.XML != nil {
		if _, err := io.Copy(w, x.xml.Body.XML); err != nil {
			return err
		}
	}

	_, err := w.Write([]byte("\n</body>\n</html>\n"))
	return err
}
