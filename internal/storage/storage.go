package storage

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"log"
	"time"

	bolt "go.etcd.io/bbolt"
)

var bucketName = []byte("usage")

type UsageRecord struct {
	Timestamp    time.Time `json:"timestamp"`
	Model        string    `json:"model"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	TotalTokens  int       `json:"total_tokens"`
	Stream       bool      `json:"stream"`
	Duration     int64     `json:"duration_ms"`
	InputPreview string    `json:"input_preview,omitempty"`
}

type Store struct {
	db *bolt.DB
}

func New(filePath string) (*Store, error) {
	db, err := bolt.Open(filePath, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, err
	}
	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucketName)
		return err
	})
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Record(r UsageRecord) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketName)
		seq, _ := b.NextSequence()
		key := make([]byte, 16)
		binary.BigEndian.PutUint64(key[:8], uint64(r.Timestamp.UnixNano()))
		binary.BigEndian.PutUint64(key[8:], seq)
		val, err := json.Marshal(r)
		if err != nil {
			return err
		}
		return b.Put(key, val)
	})
}

func (s *Store) Records() ([]UsageRecord, error) {
	var records []UsageRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketName)
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var r UsageRecord
			if err := json.Unmarshal(v, &r); err != nil {
				log.Printf("WARN: skip corrupted record: %v", err)
				continue
			}
			records = append(records, r)
		}
		return nil
	})
	return records, err
}

func (s *Store) Since(start time.Time) ([]UsageRecord, error) {
	var records []UsageRecord
	min := make([]byte, 16)
	binary.BigEndian.PutUint64(min[:8], uint64(start.UnixNano()))

	err := s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(bucketName).Cursor()
		for k, v := c.Seek(min); k != nil; k, v = c.Next() {
			var r UsageRecord
			if err := json.Unmarshal(v, &r); err != nil {
				log.Printf("WARN: skip corrupted record: %v", err)
				continue
			}
			records = append(records, r)
		}
		return nil
	})
	return records, err
}

func (s *Store) Between(start, end time.Time) ([]UsageRecord, error) {
	var records []UsageRecord
	min := make([]byte, 16)
	binary.BigEndian.PutUint64(min[:8], uint64(start.UnixNano()))
	max := make([]byte, 16)
	binary.BigEndian.PutUint64(max[:8], uint64(end.UnixNano()))
	for i := 8; i < 16; i++ {
		max[i] = 0xff
	}

	err := s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(bucketName).Cursor()
		for k, v := c.Seek(min); k != nil; k, v = c.Next() {
			if bytes.Compare(k, max) > 0 {
				break
			}
			var r UsageRecord
			if err := json.Unmarshal(v, &r); err != nil {
				log.Printf("WARN: skip corrupted record: %v", err)
				continue
			}
			records = append(records, r)
		}
		return nil
	})
	return records, err
}

func (s *Store) Count() int {
	var n int
	s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(bucketName).Cursor()
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			n++
		}
		return nil
	})
	return n
}
