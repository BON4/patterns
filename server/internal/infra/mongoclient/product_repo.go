package mongoclient

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

type ProductRepo struct {
	coll *mongo.Collection
}

func NewProductRepo(db *mongo.Database, collName string) *ProductRepo {
	return &ProductRepo{coll: db.Collection(collName)}
}

func (r *ProductRepo) GetProduct(ctx context.Context, name string) (*Product, error) {
	var p Product
	err := r.coll.FindOne(ctx, bson.E{"name", name}).Decode(&p)
	if err != nil && errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	return &p, err
}
