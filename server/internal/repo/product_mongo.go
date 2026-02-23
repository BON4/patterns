package repo

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type Product struct {
	ID    *bson.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	Name  string         `bson:"name" json:"name"`
	Price float64        `bson:"price" json:"price"`
}

type ProductMongoRepo struct {
	coll *mongo.Collection
}

func NewProductMongoRepo(db *mongo.Database, collName string) *ProductMongoRepo {
	return &ProductMongoRepo{coll: db.Collection(collName)}
}

func (r *ProductMongoRepo) GetProduct(ctx context.Context, name string) (*Product, error) {
	var p Product
	err := r.coll.FindOne(ctx, bson.M{"name": name}).Decode(&p)
	if err != nil && errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	return &p, err
}
