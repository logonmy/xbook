package models

import (
	"fmt"

	"github.com/astaxie/beego/orm"
)

//设置增减
//@param            table           需要处理的数据表
//@param            field           字段
//@param            condition       条件
//@param            incre           是否是增长值，true则增加，false则减少
//@param            step            增或减的步长
func SetIncreAndDecre(table string, field string, condition string, incre bool, step ...int) (err error) {
	mark := "-"
	if incre {
		mark = "+"
	}
	s := 1
	if len(step) > 0 {
		s = step[0]
	}
	sql := fmt.Sprintf("update %v set %v=%v%v%v where %v", table, field, field, mark, s, condition)
	_, err = orm.NewOrm().Raw(sql).Exec()
	return
}

type SitemapDocs struct {
	DocumentId   int
	DocumentName string
	Identify     string
	BookId       int
}

//站点地图数据
func SitemapData(page, listRows int) (totalRows int64, sitemaps []SitemapDocs) {
	//获取公开的项目
	var (
		books   []Book
		docs    []Document
		maps    = make(map[int]string)
		booksId []interface{}
	)

	o := orm.NewOrm()
	o.QueryTable("md_books").Filter("privately_owned", 0).Limit(100000).All(&books, "book_id", "identify")
	if len(books) > 0 {
		for _, book := range books {
			booksId = append(booksId, book.BookId)
			maps[book.BookId] = book.Identify
		}
		q := o.QueryTable("md_documents").Filter("BookId__in", booksId...)
		totalRows, _ = q.Count()
		q.Limit(listRows).Offset((page-1)*listRows).All(&docs, "document_id", "document_name", "book_id")
		if len(docs) > 0 {
			for _, doc := range docs {
				sd := SitemapDocs{
					DocumentId:   doc.DocumentId,
					DocumentName: doc.DocumentName,
					BookId:       doc.BookId,
				}
				if v, ok := maps[doc.BookId]; ok {
					sd.Identify = v
				}
				sitemaps = append(sitemaps, sd)
			}
		}
	}
	return
}

// 统计书籍分类
var counting = false

type Count struct {
	Cnt        int
	CategoryId int
}

func CountCategory() {
	if counting {
		return
	}
	counting = true
	defer func() {
		counting = false
	}()

	var count []Count

	o := orm.NewOrm()
	sql := "select count(bc.id) cnt, bc.category_id from md_book_category bc left join md_books b on b.book_id=bc.book_id where b.privately_owned=0 group by bc.category_id"
	o.Raw(sql).QueryRows(&count)
	if len(count) == 0 {
		return
	}

	var cates []Category
	tableCate := "md_category"
	o.QueryTable(tableCate).All(&cates, "id", "pid", "cnt")
	if len(cates) == 0 {
		return
	}

	var err error

	o.Begin()
	defer func() {
		if err != nil {
			o.Rollback()
		} else {
			o.Commit()
		}
	}()

	o.QueryTable(tableCate).Update(orm.Params{"cnt": 0})
	cateChild := make(map[int]int)
	for _, item := range count {
		if item.Cnt > 0 {
			cateChild[item.CategoryId] = item.Cnt
			_, err = o.QueryTable(tableCate).Filter("id", item.CategoryId).Update(orm.Params{"cnt": item.Cnt})
			if err != nil {
				return
			}
		}
	}
}
