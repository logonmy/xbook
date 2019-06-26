package models

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/astaxie/beego"
	"github.com/astaxie/beego/orm"
	"github.com/ziyoubiancheng/xbook/conf"
	"github.com/ziyoubiancheng/xbook/utils"
	"github.com/ziyoubiancheng/xbook/utils/common"
)

// Document struct.
type Document struct {
	DocumentId   int           `orm:"pk;auto;column(document_id)" json:"doc_id"`
	DocumentName string        `orm:"column(document_name);size(500)" json:"doc_name"`
	Identify     string        `orm:"column(identify);size(100);index;null;default(null)" json:"identify"` // Identify 文档唯一标识
	BookId       int           `orm:"column(book_id);type(int);index" json:"book_id"`
	ParentId     int           `orm:"column(parent_id);type(int);index;default(0)" json:"parent_id"`
	OrderSort    int           `orm:"column(order_sort);default(0);type(int);index" json:"order_sort"`
	Release      string        `orm:"column(release);type(text);null" json:"release"` // Release 发布后的Html格式内容.
	CreateTime   time.Time     `orm:"column(create_time);type(datetime);auto_now_add" json:"create_time"`
	MemberId     int           `orm:"column(member_id);type(int)" json:"member_id"`
	ModifyTime   time.Time     `orm:"column(modify_time);type(datetime);default(null);auto_now" json:"modify_time"`
	ModifyAt     int           `orm:"column(modify_at);type(int)" json:"-"`
	Version      int64         `orm:"type(bigint);column(version)" json:"version"`
	AttachList   []*Attachment `orm:"-" json:"attach"`
	Vcnt         int           `orm:"column(vcnt);default(0)" json:"vcnt"` //文档项目被浏览次数
	Markdown     string        `orm:"-" json:"markdown"`
}

// 多字段唯一键
func (m *Document) TableUnique() [][]string {
	return [][]string{
		[]string{"BookId", "Identify"},
	}
}

// TableName 获取对应数据库表名.
func (m *Document) TableName() string {
	return "documents"
}

// TableEngine 获取数据使用的引擎.
func (m *Document) TableEngine() string {
	return "INNODB"
}

func (m *Document) TableNameWithPrefix() string {
	return conf.GetDatabasePrefix() + m.TableName()
}

func NewDocument() *Document {
	return &Document{
		Version: time.Now().Unix(),
	}
}

//根据文档ID查询指定文档.
func (m *Document) Find(id int) (doc *Document, err error) {
	if id <= 0 {
		return m, ErrInvalidParameter
	}

	o := orm.NewOrm()

	err = o.QueryTable(m.TableNameWithPrefix()).Filter("document_id", id).One(m)
	if err == orm.ErrNoRows {
		return m, ErrDataNotExist
	}

	return m, nil
}

//插入和更新文档.
//存在文档id或者文档标识，则表示更新文档内容
func (m *Document) InsertOrUpdate(cols ...string) (id int64, err error) {
	o := orm.NewOrm()
	id = int64(m.DocumentId)
	m.ModifyTime = time.Now()
	m.DocumentName = strings.TrimSpace(m.DocumentName)
	if m.DocumentId > 0 { //文档id存在，则更新
		_, err = o.Update(m, cols...)
		return
	}

	var mm Document
	//直接查询一个字段，优化MySQL IO
	o.QueryTable("md_documents").Filter("identify", m.Identify).Filter("book_id", m.BookId).One(&mm, "document_id")
	if mm.DocumentId == 0 {
		m.CreateTime = time.Now()
		id, err = o.Insert(m)
		NewBook().ResetDocumentNumber(m.BookId)
	} else { //identify存在，则执行更新
		_, err = o.Update(m)
		id = int64(mm.DocumentId)
	}
	return
}

//根据指定字段查询一条文档.
func (m *Document) FindByFieldFirst(field string, v interface{}) (*Document, error) {
	o := orm.NewOrm()
	err := o.QueryTable(m.TableNameWithPrefix()).Filter(field, v).One(m)
	return m, err
}

//根据指定字段查询一条文档.
func (m *Document) FindByBookIdAndDocIdentify(BookId, Identify interface{}) (*Document, error) {
	err := orm.NewOrm().QueryTable(m.TableNameWithPrefix()).Filter("BookId", BookId).Filter("Identify", Identify).One(m)
	return m, err
}

//递归删除一个文档.
func (m *Document) RecursiveDocument(docId int) error {

	o := orm.NewOrm()
	modelStore := new(DocumentStore)

	if doc, err := m.Find(docId); err == nil {
		o.Delete(doc)
		modelStore.DeleteById(docId)
		NewDocumentHistory().Clear(docId)
	}

	var docs []*Document

	_, err := o.QueryTable(m.TableNameWithPrefix()).Filter("parent_id", docId).All(&docs)
	if err != nil {
		beego.Error("RecursiveDocument => ", err)
		return err
	}

	for _, item := range docs {
		docId := item.DocumentId
		o.QueryTable(m.TableNameWithPrefix()).Filter("document_id", docId).Delete()
		//删除document_store表的文档
		modelStore.DeleteById(docId)
		m.RecursiveDocument(docId)
	}
	return nil
}

//发布文档内容为HTML
func (m *Document) ReleaseContent(bookId int, baseUrl string) {
	// 加锁
	utils.BooksRelease.Set(bookId)
	defer utils.BooksRelease.Delete(bookId)

	var (
		o           = orm.NewOrm()
		docs        []*Document
		book        Book
		tableBooks  = "md_books"
		releaseTime = time.Now() //发布的时间戳
	)

	qs := o.QueryTable(tableBooks).Filter("book_id", bookId)
	qs.One(&book)

	//全部重新发布。查询该书籍的所有文档id
	_, err := o.QueryTable(m.TableNameWithPrefix()).Filter("book_id", bookId).Limit(20000).All(&docs, "document_id")
	if err != nil {
		beego.Error("发布失败 => ", err)
		return
	}

	ModelStore := new(DocumentStore)
	for _, item := range docs {
		content := strings.TrimSpace(ModelStore.GetFiledById(item.DocumentId, "content"))
		if len(utils.GetTextFromHtml(content)) == 0 {
			//内容为空，渲染一下文档，然后再重新获取
			utils.RenderDocumentById(item.DocumentId)
			content = strings.TrimSpace(ModelStore.GetFiledById(item.DocumentId, "content"))
		}
		item.Release = content
		attachList, err := NewAttachment().FindListByDocumentId(item.DocumentId)
		if err == nil && len(attachList) > 0 {
			content := bytes.NewBufferString("<div class=\"attach-list\"><strong>附件</strong><ul>")
			for _, attach := range attachList {
				li := fmt.Sprintf("<li><a href=\"%s\" target=\"_blank\" title=\"%s\">%s</a></li>", attach.HttpPath, attach.FileName, attach.FileName)
				content.WriteString(li)
			}
			content.WriteString("</ul></div>")
			item.Release += content.String()
		}
		_, err = o.Update(item, "release")
		if err != nil {
			beego.Error(fmt.Sprintf("发布失败 => %+v", item), err)
		}
	}

	//最后再更新时间戳
	if _, err = qs.Update(orm.Params{
		"release_time": releaseTime,
	}); err != nil {
		beego.Error(err.Error())
	}
	client := NewElasticSearchClient()
	client.RebuildAllIndex(bookId)
}

//根据项目ID查询文档列表.
func (m *Document) FindListByBookId(bookId int) (docs []*Document, err error) {
	o := orm.NewOrm()
	_, err = o.QueryTable(m.TableNameWithPrefix()).Filter("book_id", bookId).OrderBy("order_sort").All(&docs)
	return
}

//根据项目ID查询文档一级目录.
func (m *Document) GetMenuTop(bookId int) (docs []*Document, err error) {
	var docsAll []*Document
	o := orm.NewOrm()
	cols := []string{"document_id", "document_name", "member_id", "parent_id", "book_id", "identify"}
	_, err = o.QueryTable(m.TableNameWithPrefix()).Filter("book_id", bookId).Filter("parent_id", 0).OrderBy("order_sort", "document_id").Limit(5000).All(&docsAll, cols...)
	//以"."开头的文档标识，不在阅读目录显示
	for _, doc := range docsAll {
		if !strings.HasPrefix(doc.Identify, ".") {
			docs = append(docs, doc)
		}
	}
	return
}

//自动生成下一级的内容
func (m *Document) XbookAuto(bookId, docId int) (md, cont string) {
	//自动生成文档内容
	var docs []Document
	orm.NewOrm().QueryTable("md_documents").Filter("book_id", bookId).Filter("parent_id", docId).OrderBy("order_sort").All(&docs, "document_id", "document_name", "identify")
	var newCont []string //新HTML内容
	var newMd []string   //新markdown内容
	for _, doc := range docs {
		newMd = append(newMd, fmt.Sprintf(`- [%v]($%v)`, doc.DocumentName, doc.Identify))
		newCont = append(newCont, fmt.Sprintf(`<li><a href="$%v">%v</a></li>`, doc.Identify, doc.DocumentName))
	}
	md = strings.Join(newMd, "\n")
	cont = "<ul>" + strings.Join(newCont, "") + "</ul>"
	return
}

//爬虫批量采集
//@param		html				html
//@param		md					markdown内容
//@return		content,markdown	把链接替换为标识后的内容
func (m *Document) XbookCrawl(html, md string, bookId, uid int) (content, markdown string, err error) {
	var gq *goquery.Document
	content = html
	markdown = md
	project := ""
	if book, err := NewBook().Find(bookId); err == nil {
		project = book.Identify
	}
	//执行采集
	if gq, err = goquery.NewDocumentFromReader(strings.NewReader(content)); err == nil {
		//采集模式mode
		CrawlByChrome := false
		if strings.ToLower(gq.Find("mode").Text()) == "chrome" {
			CrawlByChrome = true
		}
		//内容选择器selector
		selector := ""
		if selector = strings.TrimSpace(gq.Find("selector").Text()); selector == "" {
			err = errors.New("内容选择器不能为空")
			return
		}

		// 截屏选择器
		if screenshot := strings.TrimSpace(gq.Find("screenshot").Text()); screenshot != "" {
			utils.ScreenShotProjects.Store(project, screenshot)
			defer utils.DeleteScreenShot(project)
		}

		//排除的选择器
		var exclude []string
		if excludeStr := strings.TrimSpace(gq.Find("exclude").Text()); excludeStr != "" {
			slice := strings.Split(excludeStr, ",")
			for _, item := range slice {
				exclude = append(exclude, strings.TrimSpace(item))
			}
		}

		var links = make(map[string]string) //map[url]identify

		gq.Find("a").Each(func(i int, selection *goquery.Selection) {
			if href, ok := selection.Attr("href"); ok {
				if !strings.HasPrefix(href, "$") {
					identify := utils.MD5Sub16(href) + ".md"
					links[href] = identify
				}
			}
		})

		gq.Find("a").Each(func(i int, selection *goquery.Selection) {
			if href, ok := selection.Attr("href"); ok {
				hrefLower := strings.ToLower(href)
				//以http或者https开头
				if strings.HasPrefix(hrefLower, "http://") || strings.HasPrefix(hrefLower, "https://") {
					//采集文章内容成功，创建文档，填充内容，替换链接为标识
					if retMD, err := utils.CrawlHtml2Markdown(href, 0, CrawlByChrome, 2, selector, exclude, links, map[string]string{"project": project}); err == nil {
						var doc Document
						identify := utils.MD5Sub16(href) + ".md"
						doc.Identify = identify
						doc.BookId = bookId
						doc.Version = time.Now().Unix()
						doc.ModifyAt = int(time.Now().Unix())
						doc.DocumentName = selection.Text()
						doc.MemberId = uid

						if docId, err := doc.InsertOrUpdate(); err != nil {
							beego.Error("InsertOrUpdate => ", err)
						} else {
							var ds DocumentStore
							ds.DocumentId = int(docId)
							ds.Markdown = "[TOC]\n\r\n\r" + retMD
							if err := new(DocumentStore).InsertOrUpdate(ds, "markdown", "content"); err != nil {
								beego.Error(err)
							}
						}
						selection = selection.SetAttr("href", "$"+identify)
						if _, ok := links[href]; ok {
							markdown = strings.Replace(markdown, "("+href+")", "($"+identify+")", -1)
						}
					} else {
						beego.Error(err.Error())
					}
				}
			}
		})
		content, _ = gq.Find("body").Html()
	}
	return
}

// markdown 文档拆分
func (m *Document) SplitMarkdownAndStore(seg string, markdown string, docId int) (err error) {
	var mapReplace = map[string]string{
		"${7}$": "#######",
		"${6}$": "######",
		"${5}$": "#####",
		"${4}$": "####",
		"${3}$": "###",
		"${2}$": "##",
		"${1}$": "#",
	}

	m, err = m.Find(docId)
	if err != nil {
		return
	}

	newIdentifyFmt := "spilt.%v." + m.Identify

	seg = fmt.Sprintf("${%v}$", strings.Count(seg, "#"))
	for i := 7; i > 0; i-- {
		slice := make([]string, i+1)
		k := "\n" + strings.Join(slice, "#")
		markdown = strings.Replace(markdown, k, fmt.Sprintf("\n${%v}$", i), -1)
	}
	contSlice := strings.Split(markdown, seg)

	for idx, val := range contSlice {
		var doc = NewDocument()

		if idx != 0 {
			val = seg + val
		}
		for k, v := range mapReplace {
			val = strings.Replace(val, k, v, -1)
		}

		doc.Identify = fmt.Sprintf(newIdentifyFmt, idx)
		if idx == 0 { //不需要使用newIdentify
			doc = m
		} else {
			doc.OrderSort = idx
			doc.ParentId = m.DocumentId
		}
		doc.Release = ""
		doc.BookId = m.BookId
		doc.Markdown = val
		doc.DocumentName = utils.ParseTitleFromMdHtml(common.Md2html(val))
		doc.Version = time.Now().Unix()
		doc.MemberId = m.MemberId

		if !strings.Contains(doc.Markdown, "[TOC]") {
			doc.Markdown = "[TOC]\r\n" + doc.Markdown
		}

		if docId, err := doc.InsertOrUpdate(); err != nil {
			beego.Error("InsertOrUpdate => ", err)
		} else {
			var ds = DocumentStore{
				DocumentId: int(docId),
				Markdown:   doc.Markdown,
			}
			if err := ds.InsertOrUpdate(ds, "markdown"); err != nil {
				beego.Error(err)
			}
		}

	}
	return
}
