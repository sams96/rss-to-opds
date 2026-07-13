package main

import (
	"bufio"
	"encoding/xml"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sams96/rss-to-opds/epub"

	"github.com/PuerkitoBio/goquery"
	"github.com/google/uuid"
	"github.com/mmcdole/gofeed"
	"github.com/opds-community/libopds2-go/opds1"
)

func feed(w http.ResponseWriter, r *http.Request) {
	feedURL := r.PathValue("url")
	fp := gofeed.NewParser()
	feed, _ := fp.ParseURL(feedURL)

	opds := opds1.Feed{
		ID:      uuid.NewString(),
		Title:   feed.Title,
		Updated: time.Now(),
	}

	for _, item := range feed.Items {
		pub := opds1.Entry{
			Title:   item.Title,
			ID:      item.GUID,
			Updated: item.UpdatedParsed,
			Links: []opds1.Link{
				{
					Rel:      "http://opds-spec.org/acquisition/buy",
					TypeLink: "application/epub+zip",
					Href:     fmt.Sprintf("/%s/download/%s", url.QueryEscape(feedURL), url.QueryEscape(item.GUID)),
				},
			},
		}
		opds.Entries = append(opds.Entries, pub)
		opds.TotalResults++
		opds.ItemsPerPage++
	}

	j, _ := xml.Marshal(opds)

	w.Header().Set("Content-Type", "application/atom+xml;profile=opds-catalog;kind=acquisition")
	fmt.Fprintf(w, "%s", string(j))
}

func fullContent(s *goquery.Selection) string {
	var htmlBuilder strings.Builder
	s.Each(func(_ int, s *goquery.Selection) {
		nodeHTML, _ := goquery.OuterHtml(s)
		htmlBuilder.WriteString(nodeHTML)
	})

	return htmlBuilder.String()
}

func download(w http.ResponseWriter, r *http.Request) {
	feedURL := r.PathValue("url")
	fp := gofeed.NewParser()
	feed, _ := fp.ParseURL(feedURL)

	var item *gofeed.Item
	for _, i := range feed.Items {
		if i.GUID == r.PathValue("id") {
			item = i
		}
	}
	if item == nil {
		log.Fatal("no item")
	}

	content := item.Content
	if len(content) == 0 {
		content = item.Description
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(content))
	if err != nil {
		log.Fatal(err)
	}

	doc.Find("br").Remove()

	e, err := epub.NewEpub(item.Title)
	if err != nil {
		log.Fatal(err)
	}
	if len(item.Authors) > 0 {
		e.SetAuthor(item.Authors[0].Name)
	}

	h1s := doc.Find("h1")
	if h1s.Length() == 0 {
		entireDoc := doc.Find("body").Children()
		if entireDoc.Length() > 0 {
			e.AddSection(fullContent(entireDoc), item.Title)
		}
	} else {
		firstH1 := h1s.First()
		introSection := firstH1.PrevAll()
		if introSection.Length() > 0 {
			e.AddSection(fullContent(introSection), "Introduction")
		}
	}

	h1s.Each(func(i int, h1 *goquery.Selection) {
		nextH1 := h1.NextAllFiltered("h1").First()
		section := h1.AddSelection(h1.NextUntilSelection(nextH1))
		e.AddSection(fullContent(section), h1.Text())
	})

	pw := bufio.NewWriterSize(w, 10<<16)

	w.Header().Set("Content-Type", "application/epub+zip")
	_, err = e.WriteTo(pw)
	if err != nil {
		log.Fatal(err)
	}

	pw.Flush()
}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/{url}", feed)
	mux.HandleFunc("/{url}/download/{id}", download)

	log.Fatal(http.ListenAndServe(":8080", mux))
}
