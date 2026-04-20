package db

import (
	"fmt"
	"litcontainer/pkg/logger"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.etcd.io/bbolt"
)

// BoltDB 封装了bbolt数据库连接和操作
type BoltDB struct {
	db *bbolt.DB
}

const DefaultBoltDBClientName = "default"

var BoltDBClients = make(map[string]*BoltDB)
var lock sync.Mutex
var shortConnLock sync.Mutex

func InitBoltDBClients(clientName string, dbPath string) error {
	lock.Lock()
	defer lock.Unlock()
	if _, ok := BoltDBClients[clientName]; ok {
		logger.Debug("bolt db client %s already exists", clientName)
		return nil
	}

	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create db dir %s failed: %w", dir, err)
	}

	db, err := NewBoltDB(dbPath)
	if err != nil {
		logger.Error("init blot db client %s failed: %v", clientName, err)
		panic(err)
	}
	BoltDBClients[clientName] = db
	return err
}

func GetBoltDBClient(name string) *BoltDB {
	if name == "" {
		name = DefaultBoltDBClientName
	}
	if client, ok := BoltDBClients[name]; ok {
		return client
	}
	panic(fmt.Sprintf("bolt db client %s not found", name))
}

// NewBoltDB 创建一个新的BoltDB实例
func NewBoltDB(dbPath string) (*BoltDB, error) {
	db, err := bbolt.Open(dbPath, 0600, &bbolt.Options{Timeout: 10 * time.Second})
	if err != nil {
		return nil, err
	}

	return &BoltDB{db: db}, nil
}

// WithBoltDB 使用短连接执行数据库操作
func WithBoltDB(dbPath string, fn func(*BoltDB) error) error {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create db dir %s failed: %w", dir, err)
	}

	shortConnLock.Lock()
	boltDB, err := NewBoltDB(dbPath)
	shortConnLock.Unlock()
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := boltDB.Close(); closeErr != nil {
			logger.Error("close bolt db failed: %v", closeErr)
		}
	}()

	return fn(boltDB)
}

// Close 关闭数据库连接
func (b *BoltDB) Close() error {
	return b.db.Close()
}

// CreateBucket 创建一个新的bucket
func (b *BoltDB) CreateBucket(bucketName string) error {
	return b.db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucket([]byte(bucketName))
		return err
	})
}

// CreateBucketIfNotExists 如果bucket不存在则创建
func (b *BoltDB) CreateBucketIfNotExists(bucketName string) error {
	return b.db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(bucketName))
		return err
	})
}

// DeleteBucket 删除一个bucket
func (b *BoltDB) DeleteBucket(bucketName string) error {
	return b.db.Update(func(tx *bbolt.Tx) error {
		return tx.DeleteBucket([]byte(bucketName))
	})
}

// Put 在指定bucket中存储键值对
func (b *BoltDB) Put(bucketName string, key string, value []byte) error {
	return b.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(bucketName))
		if bucket == nil {
			return fmt.Errorf("bucket %s does not exist", bucketName)
		}
		return bucket.Put([]byte(key), value)
	})
}

// Get 从指定bucket中获取值
func (b *BoltDB) Get(bucketName string, key string) ([]byte, error) {
	var value []byte
	err := b.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(bucketName))
		if bucket == nil {
			return fmt.Errorf("bucket %s does not exist", bucketName)
		}
		value = bucket.Get([]byte(key))
		return nil
	})

	if err != nil {
		return nil, err
	}

	// 返回副本，避免用户直接访问内部数据
	if value != nil {
		result := make([]byte, len(value))
		copy(result, value)
		return result, nil
	}

	return nil, nil
}

// Delete 从指定bucket中删除键值对
func (b *BoltDB) Delete(bucketName string, key string) error {
	return b.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(bucketName))
		if bucket == nil {
			return fmt.Errorf("bucket %s does not exist", bucketName)
		}
		return bucket.Delete([]byte(key))
	})
}

// GetAll 获取指定bucket中的所有键值对
func (b *BoltDB) GetAll(bucketName string) (map[string][]byte, error) {
	result := make(map[string][]byte)

	err := b.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(bucketName))
		if bucket == nil {
			return fmt.Errorf("bucket %s does not exist", bucketName)
		}

		cursor := bucket.Cursor()
		for k, v := cursor.First(); k != nil; k, v = cursor.Next() {
			// 创建值的副本，避免直接访问内部数据
			value := make([]byte, len(v))
			copy(value, v)
			result[string(k)] = value
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

// Exists 检查指定bucket中的键是否存在
func (b *BoltDB) Exists(bucketName string, key string) (bool, error) {
	var exists bool

	err := b.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(bucketName))
		if bucket == nil {
			return fmt.Errorf("bucket %s does not exist", bucketName)
		}

		value := bucket.Get([]byte(key))
		exists = value != nil
		return nil
	})

	return exists, err
}

// Count 返回指定bucket中的键值对数量
func (b *BoltDB) Count(bucketName string) (int, error) {
	var count int

	err := b.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(bucketName))
		if bucket == nil {
			return fmt.Errorf("bucket %s does not exist", bucketName)
		}

		cursor := bucket.Cursor()
		for k, _ := cursor.First(); k != nil; k, _ = cursor.Next() {
			count++
		}

		return nil
	})

	return count, err
}

// ForEach 遍历指定bucket中的所有键值对
func (b *BoltDB) ForEach(bucketName string, fn func(string, []byte) error) error {
	return b.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(bucketName))
		if bucket == nil {
			return fmt.Errorf("bucket %s does not exist", bucketName)
		}

		return bucket.ForEach(func(k, v []byte) error {
			// 创建值的副本，避免直接访问内部数据
			value := make([]byte, len(v))
			copy(value, v)
			return fn(string(k), value)
		})
	})
}

// ClearBucket 清空指定bucket中的所有数据
func (b *BoltDB) ClearBucket(bucketName string) error {
	return b.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(bucketName))
		if bucket == nil {
			// 如果bucket不存在，直接返回
			return nil
		}

		// 获取所有键
		var keysToDelete [][]byte
		cursor := bucket.Cursor()
		for k, _ := cursor.First(); k != nil; k, _ = cursor.Next() {
			keysToDelete = append(keysToDelete, make([]byte, len(k)))
			copy(keysToDelete[len(keysToDelete)-1], k)
		}

		// 删除所有键值对
		for _, key := range keysToDelete {
			if err := bucket.Delete(key); err != nil {
				return err
			}
		}

		return nil
	})
}
