/*
 * knoxite
 *     Copyright (c) 2016, Christian Muehlhaeuser <muesli@gmail.com>
 *     Copyright (c) 2016, Stefan Luecke <glaxx@glaxx.net>
 *
 *   For license see LICENSE.txt
 */

package amazons3

import (
	"bytes"
	"io/ioutil"
	"net/url"
	"strconv"
	"strings"

	"github.com/minio/minio-go"

	"github.com/knoxite/knoxite/lib"
)

// StorageAmazonS3 stores data on a remote AmazonS3
type StorageAmazonS3 struct {
	url              url.URL
	chunkBucket      string
	snapshotBucket   string
	repositoryBucket string
	region           string
	client           *minio.Client
}

func init() {
	knoxite.RegisterBackendFactory(&StorageAmazonS3{})
}

// NewBackend returns a StorageAmazonS3 backend
func (*StorageAmazonS3) NewBackend(URL url.URL) (knoxite.Backend, error) {
	ssl := true
	switch URL.Scheme {
	case "s3":
		ssl = false
	case "s3s":
		ssl = true
	default:
		panic("Invalid s3 url scheme")
	}

	if URL.User.Username() == "" {
		return &StorageAmazonS3{}, knoxite.ErrInvalidUsername
	}

	pw, pwexist := URL.User.Password()
	if !pwexist {
		return &StorageAmazonS3{}, knoxite.ErrInvalidPassword
	}

	regionAndBucketPrefix := strings.Split(URL.Path, "/")
	if len(regionAndBucketPrefix) != 3 {
		return &StorageAmazonS3{}, knoxite.ErrInvalidRepositoryURL
	}

	cl, err := minio.New(URL.Host, URL.User.Username(), pw, ssl)
	if err != nil {
		return &StorageAmazonS3{}, err
	}

	return &StorageAmazonS3{url: URL,
		client:           cl,
		region:           regionAndBucketPrefix[1],
		chunkBucket:      regionAndBucketPrefix[2] + "-chunks",
		snapshotBucket:   regionAndBucketPrefix[2] + "-snapshots",
		repositoryBucket: regionAndBucketPrefix[2] + "-repository",
	}, nil
}

// Location returns the type and location of the repository
func (backend *StorageAmazonS3) Location() string {
	return backend.url.String()
}

// Close the backend
func (backend *StorageAmazonS3) Close() error {
	return nil
}

// Protocols returns the Protocol Schemes supported by this backend
func (backend *StorageAmazonS3) Protocols() []string {
	return []string{"s3", "s3s"}
}

// Description returns a user-friendly description for this backend
func (backend *StorageAmazonS3) Description() string {
	return "Amazon S3 Storage"
}

// AvailableSpace returns the free space on this backend
func (backend *StorageAmazonS3) AvailableSpace() (uint64, error) {
	return uint64(0), knoxite.ErrAvailableSpaceUnknown
}

// LoadChunk loads a Chunk from network
func (backend *StorageAmazonS3) LoadChunk(shasum string, part, totalParts uint) (*[]byte, error) {
	fileName := shasum + "." + strconv.FormatUint(uint64(part), 10) + "_" + strconv.FormatUint(uint64(totalParts), 10)
	obj, err := backend.client.GetObject(backend.chunkBucket, fileName)
	if err != nil {
		return nil, err
	}
	data, err := ioutil.ReadAll(obj)
	return &data, err
}

// StoreChunk stores a single Chunk on network
func (backend *StorageAmazonS3) StoreChunk(shasum string, part, totalParts uint, data *[]byte) (size uint64, err error) {
	fileName := shasum + "." + strconv.FormatUint(uint64(part), 10) + "_" + strconv.FormatUint(uint64(totalParts), 10)

	if _, err = backend.client.StatObject(backend.chunkBucket, fileName); err == nil {
		// Chunk is already stored
		return 0, nil
	}

	buf := bytes.NewBuffer(*data)
	i, err := backend.client.PutObject(backend.chunkBucket, fileName, buf, "application/octet-stream")
	return uint64(i), err
}

// DeleteChunk deletes a single Chunk
func (backend *StorageAmazonS3) DeleteChunk(shasum string, part, totalParts uint) error {
	// FIXME: implement this
	return knoxite.ErrDeleteChunkFailed
}

// LoadSnapshot loads a snapshot
func (backend *StorageAmazonS3) LoadSnapshot(id string) ([]byte, error) {
	obj, err := backend.client.GetObject(backend.snapshotBucket, id)
	if err != nil {
		return nil, err
	}
	return ioutil.ReadAll(obj)
}

// SaveSnapshot stores a snapshot
func (backend *StorageAmazonS3) SaveSnapshot(id string, data []byte) error {
	buf := bytes.NewBuffer(data)
	_, err := backend.client.PutObject(backend.snapshotBucket, id, buf, "application/octet-stream")
	return err
}

// LoadChunkIndex reads the chunk-index
func (backend *StorageAmazonS3) LoadChunkIndex() ([]byte, error) {
	obj, err := backend.client.GetObject(backend.chunkBucket, knoxite.ChunkIndexFilename)
	if err != nil {
		return nil, err
	}
	return ioutil.ReadAll(obj)
}

// SaveChunkIndex stores the chunk-index
func (backend *StorageAmazonS3) SaveChunkIndex(data []byte) error {
	buf := bytes.NewBuffer(data)
	_, err := backend.client.PutObject(backend.chunkBucket, knoxite.ChunkIndexFilename, buf, "application/octet-stream")
	return err
}

// InitRepository creates a new repository
func (backend *StorageAmazonS3) InitRepository() error {
	chunkBucketExist, err := backend.client.BucketExists(backend.chunkBucket)
	if err != nil {
		return err
	}
	if !chunkBucketExist {
		err = backend.client.MakeBucket(backend.chunkBucket, backend.region)
		if err != nil {
			return err
		}
	} else {
		return knoxite.ErrRepositoryExists
	}

	snapshotBucketExist, err := backend.client.BucketExists(backend.snapshotBucket)
	if err != nil {
		return err
	}
	if !snapshotBucketExist {
		err = backend.client.MakeBucket(backend.snapshotBucket, backend.region)
		if err != nil {
			return err
		}
	} else {
		return knoxite.ErrRepositoryExists
	}

	repositoryBucketExist, err := backend.client.BucketExists(backend.repositoryBucket)
	if err != nil {
		return err
	}
	if !repositoryBucketExist {
		err = backend.client.MakeBucket(backend.repositoryBucket, backend.region)
		if err != nil {
			return err
		}
	} else {
		return knoxite.ErrRepositoryExists
	}

	return nil
}

// LoadRepository reads the metadata for a repository
func (backend *StorageAmazonS3) LoadRepository() ([]byte, error) {
	obj, err := backend.client.GetObject(backend.repositoryBucket, knoxite.RepoFilename)
	if err != nil {
		return nil, err
	}
	return ioutil.ReadAll(obj)
}

// SaveRepository stores the metadata for a repository
func (backend *StorageAmazonS3) SaveRepository(data []byte) error {
	buf := bytes.NewBuffer(data)
	_, err := backend.client.PutObject(backend.repositoryBucket, knoxite.RepoFilename, buf, "application/octet-stream")
	return err
}
