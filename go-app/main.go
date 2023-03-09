package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"

	"text/template"
	"time"

	"cloud.google.com/go/cloudsqlconn"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/stdlib"

	_ "github.com/lib/pq"
)

type WorkItem struct {
	Id           int
	UserId       int
	WorkdateTime time.Time
	TimeType     string
}

// サーバーに接続したら、それぞれの関数に振り分ける
func main() {
	http.Handle("/views/", http.StripPrefix("/views/", http.FileServer(http.Dir("../views/"))))
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/", Index)
	http.HandleFunc("/create", WorkItemCreate)
	http.HandleFunc("/edit", WorkItemEdit)
	http.HandleFunc("/update", WorkItemUpdate)
	http.ListenAndServe(":"+port, nil)
}

// データベースに接続する
func dbConn() *sql.DB {
	mustGetenv := func(k string) string {
		v := os.Getenv(k)
		if v == "" {
			log.Fatalf("Warning: %s environment variable not set.\n", k)
		}
		return v
	}

	var (
		// DB_USER と DB_IAM_USER のどちらかを定義する必要がある
		// 両方が定義されている場合、DB_IAM_USER が優先される
		dbUser                 = os.Getenv("DB_USER")                   // e.g. 'my-db-user'
		dbIAMUser              = os.Getenv("DB_IAM_USER")               // e.g. 'sa-name@project-id.iam'
		dbPwd                  = mustGetenv("DB_PASS")                  // e.g. 'my-db-password'
		dbName                 = mustGetenv("DB_NAME")                  // e.g. 'my-database'
		instanceConnectionName = mustGetenv("INSTANCE_CONNECTION_NAME") // e.g. 'project:region:instance'
		usePrivate             = os.Getenv("PRIVATE_IP")
	)

	if dbUser == "" && dbIAMUser == "" {
		log.Fatal("Warning: One of DB_USER or DB_IAM_USER must be defined")
	}

	dsn := fmt.Sprintf("user=%s password=%s database=%s", dbUser, dbPwd, dbName)
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		fmt.Print(err)
		return nil
	}
	var opts []cloudsqlconn.Option
	if dbIAMUser != "" {
		opts = append(opts, cloudsqlconn.WithIAMAuthN())
	}
	if usePrivate != "" {
		opts = append(opts, cloudsqlconn.WithDefaultDialOptions(cloudsqlconn.WithPrivateIP()))
	}
	d, err := cloudsqlconn.NewDialer(context.Background(), opts...)
	if err != nil {
		fmt.Print(err)
		return nil
	}

	// インスタンスへの接続を処理するために Cloud SQL コネクターを使用する
	config.DialFunc = func(ctx context.Context, network, instance string) (net.Conn, error) {
		return d.Dial(ctx, instanceConnectionName)
	}
	dbURI := stdlib.RegisterConnConfig(config)
	dbPool, err := sql.Open("pgx", dbURI)
	if err != nil {
		fmt.Print(err)
		return nil
	}
	return dbPool
}

type IndexData struct {
	User     int
	Day      int
	Timetype string
}

// index.htmlに表示するデータを指定する
func Index(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles("../templates/index.html")
	if err != nil {
		log.Fatal(err)
	}
	db := dbConn()

	rows, err := db.Query("SELECT * from workitems ORDER BY workdatetime ASC;")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	MapRecords := make(map[IndexData]WorkItem)

	for rows.Next() {
		var record WorkItem
		err = rows.Scan(&record.Id, &record.UserId, &record.WorkdateTime, &record.TimeType)
		index := IndexData{record.UserId, record.WorkdateTime.Day(), record.TimeType}
		if err != nil {
			log.Fatal(err)
		}
		MapRecords[index] = record
	}

	if err := rows.Err(); err != nil {
		log.Fatal(err)
	}

	tmpl.Execute(w, MapRecords)

}

// 入力されたデータを受け取り、Attendancesテーブルに保存する
func WorkItemCreate(w http.ResponseWriter, r *http.Request) {
	db := dbConn()

	if r.Method == "POST" {
		userid := r.FormValue("userid")
		workdatetime := r.FormValue("workdatetime")
		timetype := r.FormValue("timetype")

		iuser, _ := strconv.Atoi(userid)

		insert, err := db.Prepare("INSERT INTO workitems(userid, workdatetime, timetype) VALUES($1,$2,$3);")
		if err != nil {
			log.Fatal(err)
		}
		insert.Exec(iuser, workdatetime, timetype)
	}

	http.Redirect(w, r, "/", 301)

}

func WorkItemEdit(w http.ResponseWriter, r *http.Request) {
	db := dbConn()

	uId := r.URL.Query().Get("id")
	rows, err := db.Query("SELECT * from workitems WHERE id=$1;", uId)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	EditRecord := WorkItem{}
	for rows.Next() {
		var item WorkItem
		err = rows.Scan(&item.Id, &item.UserId, &item.WorkdateTime, &item.TimeType)
		if err != nil {
			log.Fatal(err)
		}
		EditRecord = item
	}

	tmpl, err := template.ParseFiles("../templates/edit.html")
	if err != nil {
		log.Fatal(err)
	}
	tmpl.Execute(w, EditRecord)
}

func WorkItemUpdate(w http.ResponseWriter, r *http.Request) {
	_, err := template.ParseFiles("../templates/edit.html")
	if err != nil {
		log.Fatal(err)
	}
	db := dbConn()

	if r.Method == "POST" {
		id := r.FormValue("id")
		workdatetime := r.FormValue("workdatetime")
		timetype := r.FormValue("timetype")

		udt, err := db.Prepare("UPDATE workitems SET workdatetime=$1,timetype=$2 WHERE id=$3;")
		if err != nil {
			log.Fatal(err)
		}
		udt.Exec(workdatetime, timetype, id)
	}

	http.Redirect(w, r, "/", 301)

}
