package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sams96/rss-to-opds/epub"

	"github.com/PuerkitoBio/goquery"
	"github.com/davidbyttow/govips/v2/vips"
	"github.com/google/uuid"
	"github.com/mmcdole/gofeed"
	"github.com/opds-community/libopds2-go/opds1"
)

type handler struct {
	log *slog.Logger
}

func (h *handler) feed(w http.ResponseWriter, r *http.Request) {
	feedURL := r.PathValue("url")
	log := h.log.With(slog.String("feed", feedURL))

	fp := gofeed.NewParser()
	feed, err := fp.ParseURL(feedURL)
	if err != nil {
		log.InfoContext(r.Context(), "failed to parse url", slog.String("error", err.Error()))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

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

	j, err := xml.Marshal(opds)
	if err != nil {
		log.ErrorContext(r.Context(), "failed to marshal xml", slog.String("error", err.Error()))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

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

func replaceExt(filename, newExt string) string {
	ext := filepath.Ext(filename)
	name := strings.TrimSuffix(filename, ext)
	return name + newExt
}

func (h *handler) download(w http.ResponseWriter, r *http.Request) {
	feedURL := r.PathValue("url")
	id := r.PathValue("id")
	log := h.log.With(slog.String("feed", feedURL), slog.String("id", id))

	fp := gofeed.NewParser()
	feed, err := fp.ParseURL(feedURL)
	if err != nil {
		log.InfoContext(r.Context(), "failed to parse url", slog.String("error", err.Error()))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var item *gofeed.Item
	for _, i := range feed.Items {
		if i.GUID == id {
			item = i
		}
	}
	if item == nil {
		log.InfoContext(r.Context(), "No item for id")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	content := item.Content
	if len(content) == 0 {
		content = item.Description
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(content))
	if err != nil {
		log.WarnContext(r.Context(), "failed to parse content", slog.String("error", err.Error()))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	doc.Find("br").Remove()

	e, err := epub.New(item.Title, w)
	if err != nil {
		log.ErrorContext(r.Context(), "failed to create epub", slog.String("error", err.Error()))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if len(item.Authors) > 0 {
		e.SetAuthor(item.Authors[0].Name)
	}

	doc.Find("img").Each(func(_ int, img *goquery.Selection) {
		src, _ := img.Attr("src")
		if !strings.HasPrefix(src, "http") {
			return
		}

		log := log.With(slog.String("image src", src))

		req, err := http.NewRequestWithContext(r.Context(), "GET", src, nil)
		if err != nil {
			log.ErrorContext(r.Context(), "failed to create request ?", slog.String("error", err.Error()))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.ErrorContext(r.Context(), "failed to get image", slog.String("error", err.Error()))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		path := strings.Split(src, "/")
		filename := url.PathEscape(replaceExt(path[len(path)-1], ".jpeg"))
		e.AddMedia(resp.Body, filename, "image/jpeg",
			func(dst io.Writer, src io.Reader) (int64, error) {
				return 0, vips.TranscodeStream(src, dst, &vips.TranscodeOptions{
					Format: vips.ImageTypeJPEG,
				})
			})

		img.SetAttr("src", filename)
	})

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

	w.Header().Set("Content-Type", "application/epub+zip")
	_, err = e.Write()
	if err != nil {
		log.ErrorContext(r.Context(), "failed to write epub", slog.String("error", err.Error()))
		return
	}

	log.DebugContext(r.Context(), "successfully served article", slog.String("title", item.Title))
}

func main() {
	vips.Startup(nil)
	defer vips.Shutdown()

	mux := http.NewServeMux()

	h := handler{
		log: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})),
	}

	mux.HandleFunc("/{url}", h.feed)
	mux.HandleFunc("/{url}/download/{id}", h.download)

	log.Fatal(http.ListenAndServe(":8080", mux))
}
