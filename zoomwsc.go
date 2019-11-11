package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

func init() {
	// Db config.
	client, err := mongo.NewClient(options.Client().ApplyURI(zunkaSiteMongoDBConnectionString))

	// Db client.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = client.Connect(ctx)
	if err != nil {
		fmt.Printf("Error. %v\n", err)
	}

	// Ping db.
	ctx, _ = context.WithTimeout(context.Background(), 2*time.Second)
	err = client.Ping(ctx, readpref.Primary())
	if err != nil {
		fmt.Printf("Error. %v\n", err)
	}

	// Products collection.
	collection := client.Database("zunka").Collection("products")

	ctx, _ = context.WithTimeout(context.Background(), 3*time.Second)
	// D: A BSON document. This type should be used in situations where order matters, such as MongoDB commands.
	// M: An unordered map. It is the same as D, except it does not preserve order.
	// A: A BSON array.
	// E: A single element inside a D.
	cur, err := collection.Find(
		ctx,
		bson.D{},
		options.Find().SetProjection(bson.D{{"storeProductTitle", true}}),
	)
	// cur, err := collection.Find(ctx, bson.D{})
	if err != nil {
		log.Fatal(err)
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		var result bson.M
		err := cur.Decode(&result)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(result)
		// do something with result....
	}
	if err := cur.Err(); err != nil {
		log.Fatal(err)
	}
	err = client.Disconnect(context.TODO())
	if err != nil {
		fmt.Printf("Error. %v\n", err)
	}
}

func main() {
	fmt.Println("Hello zoom!")
}
