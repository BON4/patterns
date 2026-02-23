package repo

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type User struct {
	UUID         string `bson:"uuid" json:"uuid"`
	IP           string `bson:"ip" json:"ip"`
	RequestCount int64  `bson:"requestCount" json:"requestCount"`
}

type UserRequestUpdate struct {
	IP           string `json:"ip"`
	RequestCount int64  `json:"requestCount"`
}

type UserMongoRepo struct {
	coll *mongo.Collection
}

func NewUserMongoRepo(db *mongo.Database, collName string) *UserMongoRepo {
	return &UserMongoRepo{coll: db.Collection(collName)}
}

func (r *UserMongoRepo) DumpRequestCounts(ctx context.Context, updates []UserRequestUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	models := make([]mongo.WriteModel, 0, len(updates))
	for _, u := range updates {
		filter := bson.M{"ip": u.IP}
		update := bson.M{"$inc": bson.M{"requestCount": u.RequestCount}}
		m := mongo.NewUpdateOneModel().
			SetFilter(filter).
			SetUpdate(update).
			SetUpsert(true)
		models = append(models, m)
	}

	opts := options.BulkWrite().SetOrdered(false)
	_, err := r.coll.BulkWrite(ctx, models, opts)
	return err
}
