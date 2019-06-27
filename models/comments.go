package models

import (
	"fmt"

	"time"

	"errors"

	"github.com/astaxie/beego"
	"github.com/astaxie/beego/orm"
)

//评论表
type Comments struct {
	Id         int
	Uid        int       `orm:"index"` //用户id
	BookId     int       `orm:"index"` //文档图书id
	Content    string    //评论内容
	TimeCreate time.Time //评论时间
}

//评分表
type Score struct {
	Id         int
	BookId     int
	Uid        int
	Score      int //评分
	TimeCreate time.Time
}

// 多字段唯一键
func (this *Score) TableUnique() [][]string {
	return [][]string{
		[]string{"Uid", "BookId"},
	}
}

//评论内容
type BookCommentsResult struct {
	Uid        int       `json:"uid"`
	Score      int       `json:"score"`
	Avatar     string    `json:"avatar"`
	Nickname   string    `json:"nickname"`
	Content    string    `json:"content"`
	TimeCreate time.Time `json:"time_create"` //评论时间
}

//获取评论内容
func (this *Comments) BookComments(p, listRows, bookid int) (comments []BookCommentsResult, err error) {
	sql := `select c.content,s.score,c.uid,c.time_create,m.avatar,m.nickname from md_comments c left join md_members m on m.member_id=c.uid left join md_score s on s.uid=c.uid and s.book_id=c.book_id where c.book_id=? order by c.id desc limit %v offset %v`
	sql = fmt.Sprintf(sql, listRows, (p-1)*listRows)
	_, err = orm.NewOrm().Raw(sql, bookid).QueryRows(&comments)
	return
}

//评分内容
type BookScoresResult struct {
	Avatar     string    `json:"avatar"`
	Nickname   string    `json:"nickname"`
	Score      string    `json:"score"`
	TimeCreate time.Time `json:"time_create"` //评论时间
}

//获取评分内容
func (this *Score) BookScores(p, listRows, bookId int) (scores []BookScoresResult, err error) {
	sql := `select s.score,s.time_create,m.avatar,m.nickname from md_score s left join md_members m on m.member_id=s.uid where s.book_id=? order by s.id desc limit %v offset %v`
	sql = fmt.Sprintf(sql, listRows, (p-1)*listRows)
	_, err = orm.NewOrm().Raw(sql, bookId).QueryRows(&scores)
	return
}

//查询用户对文档的评分
func (this *Score) BookScoreByUid(uid, bookId interface{}) int {
	var score Score
	orm.NewOrm().QueryTable("md_score").Filter("uid", uid).Filter("book_id", bookId).One(&score, "score")
	return score.Score
}

//添加评论内容

//添加评分
//score的值只能是1-5，然后需要对scorex10，50则表示5.0分
func (this *Score) AddScore(uid, bookId, score int) (err error) {
	//查询评分是否已存在
	o := orm.NewOrm()
	var scoreObj = Score{Uid: uid, BookId: bookId}
	o.Read(&scoreObj, "uid", "book_id")
	if scoreObj.Id > 0 { //评分已存在
		err = errors.New("您已给当前文档打过分了")
		return
	}

	//评分不存在，添加评分记录
	score = score * 10
	scoreObj.Score = score
	scoreObj.TimeCreate = time.Now()
	o.Insert(&scoreObj)
	if scoreObj.Id > 0 { //评分添加成功，更行当前书籍图书的评分
		//评分人数+1
		var book = Book{BookId: bookId}
		o.Read(&book, "book_id")
		if book.CntScore == 0 {
			book.CntScore = 1
			book.Score = 0
		} else {
			book.CntScore = book.CntScore + 1
		}
		book.Score = (book.Score*(book.CntScore-1) + score) / book.CntScore
		_, err = o.Update(&book, "cnt_score", "score")
		if err != nil {
			beego.Error(err.Error())
			err = errors.New("评分失败，内部错误")
		}
	}
	return
}

//添加评论
func (this *Comments) AddComments(uid, bookId int, content string) (err error) {
	var comment Comments

	//查询该用户现有的评论
	second := beego.AppConfig.DefaultInt("CommentInterval", 10)
	o := orm.NewOrm()
	o.QueryTable("md_comments").Filter("uid", uid).Filter("TimeCreate__gt", time.Now().Add(-time.Duration(second)*time.Second)).OrderBy("-Id").One(&comment, "Id")
	if comment.Id > 0 {
		return errors.New(fmt.Sprintf("您距离上次发表评论时间小于 %v 秒，请歇会儿再发。", second))
	}

	//图书被评论是量+1
	var comments = Comments{
		Uid:        uid,
		BookId:     bookId,
		Content:    content,
		TimeCreate: time.Now(),
	}

	if _, err = o.Insert(&comments); err != nil {
		beego.Error(err.Error())
		err = errors.New("发表评论失败")
		return
	}

	SetIncreAndDecre("md_books", "cnt_comment", fmt.Sprintf("book_id=%v", bookId), true)
	return
}
