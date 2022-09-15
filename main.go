package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"
	"github.com/cli/go-gh/pkg/browser"
	"github.com/cli/go-gh/pkg/tableprinter"
	"github.com/cli/go-gh/pkg/term"
	"github.com/spf13/cobra"
)

const (
	trendingBaseURL string = "https://github.com/trending"
	cutset          string = ",\n /"
)

type Trending struct {
	Owner       *string
	Name        *string
	Language    *string
	Stars       *int64
	Description *string
}

func (t *Trending) URL() string {
	return fmt.Sprintf("https://github.com/%s/%s", *t.Owner, *t.Name)
}

var openInBrowser *bool

func GetTrending(language string) ([]Trending, error) {
	res, err := http.Get(fmt.Sprintf("%s/%s", trendingBaseURL, language))
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("received non-200 response from GitHub: %+v", res)
	}

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return nil, err
	}

	trendings := []Trending{}
	doc.Find("article.Box-row").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Find("h1.h3.lh-condensed > a").Attr("href")
		if !exists {
			log.Printf("Did not find href attr on eleemnt %+v\n", s)
			return
		}

		ownerRepo := strings.Split(href, "/")
		if len(ownerRepo) != 3 {
			log.Printf("href / split is not 3 long: %+v", ownerRepo)
			return
		}

		stargazersStr := strings.Trim(
			s.Find(fmt.Sprintf("a[href=\"%s/stargazers\"]", href)).Text(),
			cutset,
		)
		var stargazers int64 = 0
		if stargazersStr != "" {
			stargazers, err = strconv.ParseInt(
				strings.ReplaceAll(stargazersStr, ",", ""), 10, 64,
			)
			if err != nil {
				log.Printf("Could not parse %v as int64", stargazersStr)
			}
		}

		lang := strings.Trim(
			s.Find("span[itemprop=\"programmingLanguage\"]").Text(), cutset,
		)

		desc := strings.Trim(
			s.Find("p.col-9").Text(), cutset,
		)

		trending := Trending{
			Owner: &ownerRepo[1],
			Name:  &ownerRepo[2],
			Stars: &stargazers,
		}

		if lang != "" {
			trending.Language = &lang
		}

		if desc != "" {
			trending.Description = &desc
		}

		trendings = append(trendings, trending)
	})

	sort.Slice(trendings, func(i, j int) bool {
		return *(trendings[i].Stars) > *(trendings[j].Stars)
	})

	return trendings, nil
}

var trendingCmd cobra.Command = cobra.Command{
	Use:   "gh-trending (language?) ...",
	Short: "Show trending repositories",
	Args:  cobra.MaximumNArgs(0),
	RunE: func(cmd *cobra.Command, args []string) error {
		trendings, err := GetTrending(args[0])
		if err != nil {
			return err
		}

		if *openInBrowser {
			b := browser.New("", os.Stdout, os.Stderr)
			wg := sync.WaitGroup{}
			for _, trending := range trendings {
				wg.Add(1)
				go func(u string) {
					defer wg.Done()
					if err := b.Browse(u); err != nil {
						panic(err)
					}
				}(trending.URL())
			}
			wg.Wait()

			return nil
		}

		width, _, err := term.FromEnv().Size()
		if err != nil {
			return err
		}

		tp := tableprinter.New(os.Stdout, term.IsTerminal(os.Stdout), width)
		for _, header := range []string{
			"OWNER", "NAME", "LANG", "URL", "STARS", "DESC",
		} {
			tp.AddField(header)
		}
		tp.EndRow()

		for _, trending := range trendings {
			tp.AddField(*trending.Owner)
			tp.AddField(*trending.Name)
			tp.AddField(func() string {
				if trending.Language != nil {
					return *trending.Language
				}
				return ""
			}())
			tp.AddField(trending.URL())
			tp.AddField(fmt.Sprint(*trending.Stars))
			tp.AddField(func() string {
				if trending.Description != nil {
					return *trending.Description
				}
				return ""
			}())
			tp.EndRow()
		}

		return tp.Render()
	},
}

func init() {
	log.SetOutput(os.Stderr)
	openInBrowser = trendingCmd.PersistentFlags().BoolP(
		"web", "w", false, "Open in web browser",
	)
}

func main() {
	if err := trendingCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
