package main

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

var client *mongo.Client
var err error
var logPath, xmlPath string

// Development mode.
var dev bool
var initTime time.Time

// Just to create correct xml structure.
type _products struct {
	XMLName xml.Name `xml:"PRODUTOS"`
	A       []product
}

type product struct {
	XMLName          xml.Name           `xml:"PRODUTO"`
	ObjectID         primitive.ObjectID `bson:"_id,omitempty" xml:"-"`
	ID               string             `xml:"CODIGO"`
	Name             string             `bson:"storeProductTitle" xml:"NOME"`
	Department       string             `bson:"" xml:"DEPARTAMENTO"`
	Category         string             `bson:"storeProductCategory" xml:"SUBDEPARTAMENTO"`
	Detail           string             `bson:"storeProductDetail" xml:"DESCRICAO"`
	TechInfo         string             `bson:"storeProductTechnicalInformation" xml:"-"` // To get get ean.
	PriceFloat64     float64            `bson:"storeProductPrice" xml:"-"`
	Price            string             `bson:"" xml:"PRECO"`
	PriceFrom        string             `bson:"" xml:"PRECO_DE"`
	InstallmentQtd   int                `bson:"" xml:"NPARCELA"`
	InstallmentValue string             `bson:"" xml:"VPARCELA"`
	Url              string             `bson:"" xml:"URL"`
	UrlImage         string             `bson:"" xml:"URL_IMAGEM"`
	MPC              string             `bson:"" xml:"MPC"`    // MPC – (Manufacturer Part Number)
	EAN              string             `bson:"ean" xml:"EAN"` // EAN – (European Article Number)
	SKU              string             `bson:"" xml:"SKU"`    // SKU – (Stock Keeping Unit)
	Images           []string           `bson:"images" xml:"-"`
}

func init() {
	initTime = time.Now()
	// Path for log.
	zunkaPathLog := os.Getenv("ZUNKAPATH")
	if zunkaPathLog == "" {
		panic("ZUNKAPATH not defined.")
	}
	logPath := path.Join(zunkaPathLog, "log", "zoom")
	// Path for xml.
	zunkaPathXML := os.Getenv("ZUNKA_SITE_PATH")
	if zunkaPathXML == "" {
		panic("ZUNK_SITE_APATH not defined.")
	}
	xmlPath = path.Join(zunkaPathXML, "dist/xml/zoom")
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
	// log.Printf("*** Testing zoom api ****")
	// apiGetProducts()
	// return

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

	timeToProcess := time.Since(initTime)
	log.Printf("Time to process %.0fms\n\n", float64(timeToProcess)/1e6)
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
	findOptions.SetProjection(bson.D{
		{"storeProductTitle", true},
		{"storeProductCategory", true},
		{"storeProductDetail", true},
		{"storeProductTechnicalInformation", true},
		{"storeProductPrice", true},
		{"ean", true},
		{"images", true},
		{"dealerName", true},
	})
	// todo - comment.
	// findOptions.SetLimit(1)
	cur, err := collection.Find(ctxFind, filter, findOptions)
	checkFatalError(err)

	defer cur.Close(ctxFind)
	for cur.Next(ctxFind) {
		// var result bson.M
		result := product{
			Department:     "Informática",
			InstallmentQtd: 3,
		}
		err := cur.Decode(&result)
		checkFatalError(err)
		// Mounted fields.
		// ID.
		result.ID = result.ObjectID.Hex()
		// EAN.
		if result.EAN == "" {
			result.EAN = findEan(result.TechInfo)
		}
		// Price from.
		result.Price = fmt.Sprintf("%.2f", result.PriceFloat64)
		result.Price = strings.ReplaceAll(result.Price, ".", ",")
		result.PriceFrom = result.Price
		// Installments.
		result.InstallmentValue = fmt.Sprintf("%.2f", float64(int((result.PriceFloat64/3)*100))/100)
		result.InstallmentValue = strings.ReplaceAll(result.InstallmentValue, ".", ",")
		result.Url = "https://www.zunka.com.br/product/" + result.ID
		// Images.
		if len(result.Images) > 0 {
			result.UrlImage = "https://www.zunka.com.br/img/" + result.ID + "/" + result.Images[0]
		} else {
			result.UrlImage = ""
		}
		// log.Println(result)
		// log.Println("EAN:", findEan(result.TechInfo))
		// log.Println("TechInfo:", result.TechInfo)
		// log.Println("ObjectID:", result.ObjectID)
		// log.Println("ObjectID (string):", result.ObjectID.Hex())
		// log.Println("ID:", result.ID)
		// log.Println("Name:", result.Name)
		// log.Println("Detail:", result.Detail)
		// log.Println("Category:", result.Category)
		// log.Println("Price:", result.Price)
		// log.Println("Images:", result.Images)
		results = append(results, result)
	}
	if err := cur.Err(); err != nil {
		log.Fatal(err)
	}
	return results
}

func findEan(s string) string {
	lines := strings.Split(s, "\n")
	// (?i) case-insensitive flag.
	r := regexp.MustCompile(`(?i).*ean.*`)
	for _, line := range lines {
		if r.MatchString(line) {
			return strings.TrimSpace(strings.Split(line, ";")[1])
		}
	}
	return ""
}

func saveXML(products []product) {
	updateXMLFile := true
	fileNameSent := "zoom-produtos.xml"
	fileNameNew := "zoom-produtos-" + time.Now().Format("2006-nov-02-150405") + ".xml"

	prods := _products{
		A: products,
	}
	// Create xml.
	xmlFile, _ := xml.MarshalIndent(prods, "", "    ")
	// Add xml header.
	xmlFile = []byte(xml.Header + string(xmlFile))
	// Save with current time name.
	err := ioutil.WriteFile(path.Join(xmlPath, fileNameNew), xmlFile, 0644)
	checkFatalError(err)

	// Check if new file is different from last sent.
	xmlFileSent, err := ioutil.ReadFile(path.Join(xmlPath, fileNameSent))
	if err != nil {
		log.Println("XML file not exist.")
	} else {
		if bytes.Equal(xmlFile, xmlFileSent) {
			updateXMLFile = false
			log.Println("XML not changed.")
		}
	}
	// Save xml file.
	log.Printf("Saving XML file %v ...", path.Join(xmlPath, fileNameNew))
	// Send xml file to zoom webservice.
	if updateXMLFile {
		// Save xml as last modified.
		log.Printf("Saving XML file %v ...", path.Join(xmlPath, fileNameSent))
		err = ioutil.WriteFile(path.Join(xmlPath, fileNameSent), xmlFile, 0644)
		checkFatalError(err)
	}
}

func apiGetProducts() {
	// Request products.
	client := &http.Client{}
	req, err := http.NewRequest("GET", "http://merchant.zoom.com.br/api/merchant/products", nil)
	req.Header.Set("Content-Type", "application/json")
	checkFatalError(err)

	// Devlopment.
	req.SetBasicAuth("zoomteste_zunka", "H2VA79Ug4fjFsJb")
	// Production.
	// req.SetBasicAuth("zunka_informatica*", "h8VbfoRoMOSgZ2B")
	res, err := client.Do(req)
	checkFatalError(err)

	defer res.Body.Close()
	checkFatalError(err)

	// Result.
	resBody, err := ioutil.ReadAll(res.Body)
	checkFatalError(err)
	// No 200 status.
	if res.StatusCode != 200 {
		log.Fatalf("Error ao solicitar a criação do produtos no servidor zoom.\n\nstatus: %v\n\nbody: %v", res.StatusCode, string(resBody))
		return
	}
	// Log body result.
	log.Printf("body: %s", string(resBody))
}

func checkFatalError(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
