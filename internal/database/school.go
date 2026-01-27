package repository

import (
	"DarkCS/entity"
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const schoolsCollection = "schools"

// GetSchoolByCode retrieves a school by its deep link code.
func (m *MongoDB) GetSchoolByCode(ctx context.Context, code string) (*entity.School, error) {
	connection, err := m.connect()
	if err != nil {
		return nil, err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(schoolsCollection)

	filter := bson.D{{"code", code}, {"active", true}}

	var school entity.School
	err = collection.FindOne(ctx, filter).Decode(&school)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}

	return &school, nil
}

// GetSchoolByID retrieves a school by its ID.
func (m *MongoDB) GetSchoolByID(ctx context.Context, id string) (*entity.School, error) {
	connection, err := m.connect()
	if err != nil {
		return nil, err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(schoolsCollection)

	filter := bson.D{{"_id", id}}

	var school entity.School
	err = collection.FindOne(ctx, filter).Decode(&school)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}

	return &school, nil
}

// GetAllActiveSchools retrieves all active schools.
func (m *MongoDB) GetAllActiveSchools(ctx context.Context) ([]entity.School, error) {
	connection, err := m.connect()
	if err != nil {
		return nil, err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(schoolsCollection)

	filter := bson.D{{"active", true}}
	opts := options.Find().SetSort(bson.D{{"name", 1}})

	cursor, err := collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var schools []entity.School
	if err = cursor.All(ctx, &schools); err != nil {
		return nil, err
	}

	return schools, nil
}

// UpsertSchool inserts or updates a school.
func (m *MongoDB) UpsertSchool(ctx context.Context, school *entity.School) error {
	connection, err := m.connect()
	if err != nil {
		return err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(schoolsCollection)

	filter := bson.D{{"_id", school.ID}}
	update := bson.D{{"$set", school}}
	opts := options.Update().SetUpsert(true)

	_, err = collection.UpdateOne(ctx, filter, update, opts)
	return err
}

// DeleteSchool deletes a school by ID (soft delete - sets active to false).
func (m *MongoDB) DeleteSchool(ctx context.Context, id string) error {
	connection, err := m.connect()
	if err != nil {
		return err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(schoolsCollection)

	filter := bson.D{{"_id", id}}
	update := bson.D{{"$set", bson.D{{"active", false}}}}

	_, err = collection.UpdateOne(ctx, filter, update)
	return err
}

// GetAllSchools retrieves all schools regardless of active status, sorted by name.
func (m *MongoDB) GetAllSchools(ctx context.Context) ([]entity.School, error) {
	connection, err := m.connect()
	if err != nil {
		return nil, err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(schoolsCollection)

	opts := options.Find().SetSort(bson.D{{"name", 1}})

	cursor, err := collection.Find(ctx, bson.D{}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var schools []entity.School
	if err = cursor.All(ctx, &schools); err != nil {
		return nil, err
	}

	return schools, nil
}

// GetInactiveSchools retrieves all inactive schools, sorted by name.
func (m *MongoDB) GetInactiveSchools(ctx context.Context) ([]entity.School, error) {
	connection, err := m.connect()
	if err != nil {
		return nil, err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(schoolsCollection)

	filter := bson.D{{"active", false}}
	opts := options.Find().SetSort(bson.D{{"name", 1}})

	cursor, err := collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var schools []entity.School
	if err = cursor.All(ctx, &schools); err != nil {
		return nil, err
	}

	return schools, nil
}

// SetSchoolActive sets the active status of a school by ID.
func (m *MongoDB) SetSchoolActive(ctx context.Context, id string, active bool) error {
	connection, err := m.connect()
	if err != nil {
		return err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(schoolsCollection)

	filter := bson.D{{"_id", id}}
	update := bson.D{{"$set", bson.D{{"active", active}}}}

	_, err = collection.UpdateOne(ctx, filter, update)
	return err
}

// CountActiveSchools returns the count of active schools.
func (m *MongoDB) CountActiveSchools(ctx context.Context) (int64, error) {
	connection, err := m.connect()
	if err != nil {
		return 0, err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(schoolsCollection)

	filter := bson.D{{"active", true}}

	count, err := collection.CountDocuments(ctx, filter)
	if err != nil {
		return 0, err
	}

	return count, nil
}
