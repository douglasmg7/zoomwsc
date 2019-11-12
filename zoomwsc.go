package main

import (
	"context"
	"log"
	"time"

	"encoding/xml"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strings"
)

var client *mongo.Client
var err error
var logPath, xmlPath string

// Development mode.
var dev bool
var initTime time.Time

type product struct {
	Name       string `bson:"storeProductTitle" xml:"title"`
	DealerName string `bson:"dealerName" xml:"dealer"`
}

func init() {
	initTime = time.Now()
	// Path.
	zunkaPath := os.Getenv("ZUNKAPATH")
	if zunkaPath == "" {
		panic("ZUNKAPATH not defined.")
	}
	logPath := path.Join(zunkaPath, "log")
	xmlPath = path.Join(zunkaPath, "xml/zoom")
	// Create path.
	os.MkdirAll(logPath, os.ModePerm)
	os.MkdirAll(xmlPath, os.ModePerm)

	// Log file.
	logFile, err := os.OpenFile(path.Join(logPath, "zoomwsc.log"), os.O_CREATE|os.O_APPEND|os.O_RDWR, 0666)
	if err != nil {
		panic(err)
	}

	// Log configuration.
	mw := io.MultiWriter(os.Stdout, logFile)
	log.SetOutput(mw)
	log.SetFlags(log.Ldate | log.Lmicroseconds)

	// Run mode.
	mode := "production"
	if len(os.Args) > 1 && strings.HasPrefix(os.Args[1], "dev") {
		dev = true
		mode = "development"
	}

	// Log start.
	log.Printf("*** Starting zoomwsc in %v mode (version %s) ***\n", mode, version)
}

func main() {
	// MongoDB config.
	client, err = mongo.NewClient(options.Client().ApplyURI(zunkaSiteMongoDBConnectionString))
	// MongoDB client.
	ctxClient, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = client.Connect(ctxClient)
	checkFatalError(err)

	// Ping mongoDB.
	ctxPing, _ := context.WithTimeout(context.Background(), 2*time.Second)
	err = client.Ping(ctxPing, readpref.Primary())
	checkFatalError(err)

	// Get all products to commercialize.
	products := getProdutcts()
	log.Printf("%v products to be commercialized.", len(products))
	// log.Println(products[0])
	saveXML(products)

	// Disconnect from mongoDB.
	err = client.Disconnect(ctxClient)
	checkFatalError(err)

	log.Printf("Time to process %v", time.Since(initTime))
}

// Get all products to commercialize.
func getProdutcts() (results []product) {
	collection := client.Database("zunka").Collection("products")

	ctxFind, _ := context.WithTimeout(context.Background(), 3*time.Second)
	// D: A BSON document. This type should be used in situations where order matters, such as MongoDB commands.
	// M: An unordered map. It is the same as D, except it does not preserve order.
	// A: A BSON array.
	// E: A single element inside a D.
	// options.Find().SetProjection(bson.D{{"storeProductTitle", true}, {"_id", false}}),
	// {'storeProductCommercialize': true, 'storeProductTitle': {$regex: /\S/}, 'storeProductQtd': {$gt: 0}, 'storeProductPrice': {$gt: 0}};
	filter := bson.D{
		{"storeProductCommercialize", true},
		{"storeProductQtd", bson.D{
			{"$gt", 0},
		}},
		{"storeProductPrice", bson.D{
			{"$gt", 0},
		}},
		{"storeProductTitle", bson.D{
			{"$regex", `\S`},
		}},
	}
	findOptions := options.Find()
	findOptions.SetProjection(bson.D{{"storeProductTitle", true}, {"dealerName", true}, {"_id", false}})
	// findOptions.SetLimit(10)
	cur, err := collection.Find(ctxFind, filter, findOptions)
	checkFatalError(err)

	defer cur.Close(ctxFind)
	for cur.Next(ctxFind) {
		// var result bson.M
		result := product{}

		err := cur.Decode(&result)
		checkFatalError(err)

		// log.Println(result)
		results = append(results, result)
	}
	if err := cur.Err(); err != nil {
		log.Fatal(err)
	}
	return results
}

func saveXML(products []product) {
	xmlFile, _ := xml.MarshalIndent(products, "", " ")
	fileName := "zoom-products-" + time.Now().Format("2006-nov-02-150405") + ".xml"
	err := ioutil.WriteFile(path.Join(xmlPath, fileName), xmlFile, 0644)
	checkFatalError(err)
}

func checkFatalError(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
