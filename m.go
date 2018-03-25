package main

import (
	"bufio"
	"context"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/sync/errgroup"

	"github.com/PuerkitoBio/goquery"
)

const (
	start = 1
	end   = 5000
	gocn  = "https://gocn.io/question/"
	enter = "\r"
)

type article struct {
	t    int64 //时间戳  用于排序
	body string
}

//
type sortArticles []article

func (c sortArticles) Len() int {
	return len(c)
}
func (c sortArticles) Swap(i, j int) {
	c[i], c[j] = c[j], c[i]
}
func (c sortArticles) Less(i, j int) bool {
	return c[i].t < c[j].t
}

//
func init() {
	log.SetFlags(log.Lshortfile)
}
func main() {
	t1 := time.Now()
	log.Println("begin to get")

	var (
		gocnUrls  = make(chan string, 100)
		results   = make(chan article, 100)
		writeDone = make(chan struct{})
		//并发http去请求  不限制会有很多time out
		worker = make(chan struct{}, 100)
	)
	go func() {
		defer close(gocnUrls)
		for i := start; i < end; i++ {
			url := gocn + strconv.FormatInt(int64(i), 10)
			gocnUrls <- url
		}
	}()
	//实际未做超时处理
	ctx, _ := context.WithTimeout(context.Background(), 10000*time.Second)
	errg, ctx := errgroup.WithContext(ctx)
	//接收所有的  用于排序
	var allArtile sortArticles
	go func() {
		for r := range results {
			allArtile = append(allArtile, r)
		}
		writeDone <- struct{}{}
	}()

	//
	for v := range gocnUrls {
		worker <- struct{}{}
		url := v
		errg.Go(func() error {
			defer func() {
				<-worker
			}()
			//goquery 直接用的http.Get 没有超时设置  可以自己读出
			//然后用 goquery.NewDocumentFromReader(r io.Reader)
			doc, err := goquery.NewDocument(url)
			if err != nil {
				//log.Println("err:", err)
				return err
			}
			//找到标题
			var (
				result = make([]string, 0)
				//有的文章title在 列表一起
				titleFind = false
				// 时间转换结果
				tunix = int64(0)
				b     = false
			)
			title := doc.Find("title").Text()
			if !strings.Contains(title, "每日新闻") {
				//未找到title
			} else {
				//找到title
				titleFind = true
				tunix, b = parseTitleDate(title)
				if !b {
					log.Println("url parse title date err:", url)
					return nil
				}
				result = append(result, url, title)
			}

			//内容格式一般有两种  第一种找不到用第二种
			var findFirst = false
			doc.Find("ol li").Each(func(i int, s *goquery.Selection) {
				t := s.Text()
				result = append(result, t)
				findFirst = true
			})
			if !findFirst {
				doc.Find(".content").Find("p").Each(func(i int, s *goquery.Selection) {
					t := s.Text()
					result = append(result, t)
				})
			}
			//未找到  返回
			if len(result) < 1 {
				return nil
			}
			//如果第一次未找到title
			if !titleFind {
				tunix, b = parseTitleDate(result[0])
				if !b {
					return nil
				}
			}
			//组装
			body := strings.Join(result, enter) + enter + enter
			var one article
			one.t = tunix
			one.body = body
			results <- one
			return nil
		})
	}
	//等待完成
	errg.Wait()
	close(results)
	<-writeDone
	//
	log.Println("len allArtile", len(allArtile))
	//排序
	sort.Sort(allArtile)
	//
	f, err := os.OpenFile("gocn.txt", os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0666)
	if err != nil {
		log.Fatal(err)
	}
	w := bufio.NewWriter(f)
	for _, v := range allArtile {
		w.WriteString(v.body)
	}
	err = w.Flush()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("success to get , cost %f s", time.Now().Sub(t1).Seconds())
}

//处理发布时间  排序用  网上有开源库可用
func parseTitleDate(title string) (int64, bool) {
	//左括号 注意 有中文括号
	n1 := strings.Index(title, "(")
	if n1 == -1 {
		n1 = strings.Index(title, "（")
	}
	if n1 == -1 {
		//log.Println("n1 == -1")
		return 0, false
	}
	//右括号
	n2 := strings.Index(title, ")")
	if n2 == -1 {
		n2 = strings.Index(title, "）")
	}
	if n2 == -1 {
		//log.Println("n2 == -1")
		return 0, false
	}
	dstring := title[n1+1 : n2]
	//处理2017-2-3问题
	sp := strings.Split(dstring, "-")
	if len(sp) < 3 {
		//log.Println("len(sp) < 3")
		return 0, false
	}
	year := strings.TrimSpace(sp[0])
	month := strings.TrimSpace(sp[1])
	day := strings.TrimSpace(sp[2])
	if len(year) > 4 {
		year = year[len(year)-4:]
	}
	if len(month) == 1 {
		month = "0" + month
	}
	if len(day) == 1 {
		day = "0" + day
	}
	tstring := year + "-" + month + "-" + day
	t, err := time.Parse("2006-01-02", tstring)
	if err != nil {
		//log.Println("time.Parse err:", err)
		return 0, false
	}
	return t.Unix(), true
}
