package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"encoding/json"
	"net/http"

	"database/sql"

	_ "github.com/go-sql-driver/mysql"
	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
)

type City struct {
	Id          string  `json:"id"`
	Name        string  `json:"name"`
	CountryCode string  `json:"country_code"`
	Lat         float64 `json:"lat"`
	Lng         float64 `json:"lng"`
	Country     Country `json:"country"`
	State       State   `json:"state"`
}

type Country struct {
	Id        string `json:"id"`
	Name      string `json:"name"`
	Iso2      string `json:"iso2"`
	Phonecode string `json:"phonecode"`
	Native    string `json:"native"`
	Emoji     string `json:"emoji"`
}

type State struct {
	Id   string `json:"id"`
	Name string `json:"name"`
	Iso2 string `json:"iso2"`
}

type CacheKey struct {
	Query string
}

type CacheData struct {
	Data []City
}

var cacheDataMap map[CacheKey]CacheData

func errorResponse(w http.ResponseWriter, message string) {
	resp := make(map[string]string)
	resp["status"] = "error"
	resp["error"] = message
	jsonResp, err := json.Marshal(resp)

	if err != nil {
		log.Fatalf("Error happened in JSON marshal. Err: %s", err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusBadRequest)
	w.Write(jsonResp)
	return
}

func getStatus(w http.ResponseWriter, r *http.Request) {
	resp := make(map[string]string)
	resp["message"] = "Server is up and running."
	resp["status"] = "ok"
	resp["data"] = "https://github.com/dr5hn/countries-states-cities-database"
	jsonResp, err := json.Marshal(resp)

	if err != nil {
		log.Fatalf("Error happened in JSON marshal. Err: %s", err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Write(jsonResp)
}

func GetCacheData(key CacheKey) (*CacheData, error) {
	data := cacheDataMap[key]
	return &data, nil
}

func getCityByQuery(w http.ResponseWriter, r *http.Request) {
	db_host := os.Getenv("DB_HOST")
	db_port := os.Getenv("DB_PORT")
	db_username := os.Getenv("DB_USER")
	db_password := os.Getenv("DB_PASSWORD")
	db_name := os.Getenv("DB_DATABASE")

	connection_src := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", db_username, db_password, db_host, db_port, db_name)
	db, err := sql.Open("mysql", connection_src)

	if err != nil {
		errorResponse(w, err.Error())
		return
	}

	defer db.Close()

	query := r.URL.Query().Get("q")
	if query == "" {
		query = r.URL.Query().Get("query")
	}

	limit := r.URL.Query().Get("l")
	if limit == "" {
		limit = r.URL.Query().Get("limit")
	}

	if limit == "" {
		limit = "10"
	}

	if query == "" || len(query) < 3 || len(query) > 50 {
		errorResponse(w, "query_is_empty")
		return
	}

	if match, _ := regexp.MatchString(`^[\p{L} -]+$`, query); !match {
		errorResponse(w, "invalid_query_parameter")
		return
	}

	query = strings.TrimSpace(query)

	cache_size := os.Getenv("CACHE_SIZE")
	cache_size_int, err := strconv.Atoi(cache_size)

	if len(cacheDataMap) > cache_size_int {
		cacheDataMap = make(map[CacheKey]CacheData)
	}

	key := CacheKey{Query: query}
	data, err := GetCacheData(key)

	if err != nil {
		errorResponse(w, err.Error())
		return
	}

	if data.Data != nil {
		jsonResp, err := json.Marshal(data.Data)

		if err != nil {
			errorResponse(w, err.Error())
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Write(jsonResp)
		return
	}

	keyword := fmt.Sprintf("'%s%%'", query)
	db_query := fmt.Sprintf(`SELECT cities.id, cities.name, cities.country_code, cities.latitude, cities.longitude, cities.country_id, countries.name as c_name, countries.iso2 as c_iso2, countries.phonecode as c_phonecode, countries.native as c_native, countries.emoji as c_emoji, states.id as s_id, states.name as s_name, states.iso2 as s_iso2 FROM cities INNER JOIN countries ON cities.country_id=countries.id RIGHT JOIN states ON cities.state_id=states.id WHERE cities.name LIKE %s LIMIT %s`, keyword, limit)
	rows, err := db.Query(db_query)
	cities := []City{}

	if err != nil {
		errorResponse(w, err.Error())
		return
	}

	for rows.Next() {
		var id sql.NullString
		var name sql.NullString
		var country_code sql.NullString
		var country_id sql.NullString
		var latitude sql.NullFloat64
		var longitude sql.NullFloat64
		var c_name sql.NullString
		var c_iso2 sql.NullString
		var c_phonecode sql.NullString
		var c_native sql.NullString
		var c_emoji sql.NullString
		var s_id sql.NullString
		var s_name sql.NullString
		var s_iso2 sql.NullString

		if err := rows.Scan(&id, &name, &country_code, &latitude, &longitude, &country_id, &c_name, &c_iso2, &c_phonecode, &c_native, &c_emoji, &s_id, &s_name, &s_iso2); err != nil {
			errorResponse(w, err.Error())
			return
		}

		state := State{s_id.String, s_name.String, s_iso2.String}
		country := Country{country_id.String, c_name.String, c_iso2.String, c_phonecode.String, c_native.String, c_emoji.String}
		city := &City{id.String, name.String, country_code.String, latitude.Float64, longitude.Float64, country, state}

		cities = append(cities, *city)
	}

	if len(cities) == 0 {
		errorResponse(w, "cities_not_found")
		return
	}

	cacheDataMap[key] = CacheData{cities}

	json, err := json.Marshal(cities)
	if err != nil {
		errorResponse(w, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Write(json)
}

func main() {
	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		log.Fatal(err)
	}

	environmentPath := filepath.Join(dir, ".env")
	err = godotenv.Load(environmentPath)
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	app_port := os.Getenv("APP_PORT")

	cacheDataMap = make(map[CacheKey]CacheData)

	time := fmt.Sprintf("%s", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Printf("Starting server on port %s...\nStart time: %s\n", app_port, time)

	router := mux.NewRouter().StrictSlash(true)
	router.HandleFunc("/", getStatus).Methods("GET")
	router.HandleFunc("/city", getCityByQuery).Methods("GET")

	port := fmt.Sprintf(":%s", app_port)
	log.Fatal(http.ListenAndServe(port, router))
}
