package strife

import "cloud.google.com/go/firestore"

func createDoc(collectionID, docID string, data map[string]interface{}) (*firestore.WriteResult, error) {

	wr, err := client.Collection(collectionID).Doc(docID).Set(ctx, data)

	return wr, err
}

func updateDoc(collectionID, docID string, data map[string]interface{}) (*firestore.WriteResult, error) {
	wr, err := client.Collection(collectionID).Doc(docID).Set(ctx, data, firestore.MergeAll)

	return wr, err
}

func updateServers(serverID string, data map[string]interface{}) (*firestore.WriteResult, error) {
	wr, err := client.Collection("servers").Doc(serverID).Set(ctx, data, firestore.MergeAll)

	return wr, err
}
