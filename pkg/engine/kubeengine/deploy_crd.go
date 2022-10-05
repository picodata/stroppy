package kubeengine

import (
	"context"
	"encoding/json"
	"time"

	"github.com/pkg/errors"
	ydbApi "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
	"k8s.io/kubectl/pkg/scheme"
)

const (
	storagePluralName  = "storages"
	databasePluralName = "databases"
)

type YDBV1Alpha1Interface interface {
	Storages(namespace string) StorageInterface
	Databases(namespace string) DatabaseInterface
}

type YDBV1Alpha1Client struct {
	client *rest.RESTClient
}

func NewForConfig(cfg *rest.Config) (*YDBV1Alpha1Client, error) {
	var (
		err       error
		ydbClient *rest.RESTClient
	)

	if err = ydbApi.AddToScheme(scheme.Scheme); err != nil {
		return nil, errors.Wrap(err, "failed to add scheme to YDB v1alpha1")
	}

	config := *cfg
	config.GroupVersion = &ydbApi.GroupVersion
	config.APIPath = "/apis"
	config.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	config.UserAgent = rest.DefaultKubernetesUserAgent()

	if ydbClient, err = rest.RESTClientFor(&config); err != nil {
		return nil, errors.Wrap(err, "failed to create rest client for YDBV1Alpha1")
	}

	return &YDBV1Alpha1Client{client: ydbClient}, nil
}

func (ydbClient *YDBV1Alpha1Client) YDBV1Alpha1() YDBV1Alpha1Interface {
	return ydbClient
}

type StorageInterface interface {
	Get(context.Context, string, *metav1.GetOptions) (*ydbApi.Storage, error)
	List(context.Context, *metav1.ListOptions) (*ydbApi.StorageList, error)
	Watch(context.Context, *metav1.ListOptions) (watch.Interface, error)
	Create(context.Context, *ydbApi.Storage, *metav1.CreateOptions) (*ydbApi.Storage, error)
	Delete(context.Context, string, *metav1.DeleteOptions) error
	Patch(
		context.Context,
		string,
		types.PatchType,
		[]byte,
		*metav1.PatchOptions,
		...string,
	) (*ydbApi.Storage, error)
	Apply(context.Context, *ydbApi.Storage, *metav1.ApplyOptions) (*ydbApi.Storage, error)
}

type StorageClient struct {
	client rest.Interface
	ns     string
}

func (ydbClient *YDBV1Alpha1Client) Storages(namespace string) StorageInterface {
	return &StorageClient{
		client: ydbClient.client,
		ns:     namespace,
	}
}

var (
	errStorageMustNotBeNull     = errors.New("storage provided to Apply must not be nil")
	errStorageNameMustNotBeNull = errors.New("storage.Name must be provided to Apply")
)

// Get takes name of the storage, and returns the corresponding storage object,
// and an error if there is any.
func (storageClient *StorageClient) Get(
	ctx context.Context,
	name string,
	options *metav1.GetOptions,
) (*ydbApi.Storage, error) {
	var err error
	result := &ydbApi.Storage{} //nolint

	if err = storageClient.client.Get().
		Namespace(storageClient.ns).
		Resource(storagePluralName).
		Name(name).
		VersionedParams(options, scheme.ParameterCodec).
		Do(ctx).
		Into(result); err != nil {
		return nil, errors.Wrap(err, "failed to get storage")
	}

	return result, nil
}

// List takes label and field selectors,
// and returns the list of Storages that match those selectors.
func (storageClient *StorageClient) List(
	ctx context.Context,
	opts *metav1.ListOptions,
) (*ydbApi.StorageList, error) {
	var (
		err     error
		timeout time.Duration
	)

	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}

	result := &ydbApi.StorageList{} //nolint

	if err = storageClient.client.Get().
		Namespace(storageClient.ns).
		Resource(storagePluralName).
		VersionedParams(opts, scheme.ParameterCodec).
		Timeout(timeout).
		Do(ctx).
		Into(result); err != nil {
		return nil, errors.Wrap(err, "failed to get list of storages")
	}

	return result, nil
}

// Watch returns a watch.Interface that watches the requested Storages.
func (storageClient *StorageClient) Watch(
	ctx context.Context,
	opts *metav1.ListOptions,
) (watch.Interface, error) {
	var (
		err            error
		timeout        time.Duration
		watchInterface watch.Interface
	)

	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}

	opts.Watch = true

	if watchInterface, err = storageClient.client.Get().
		Namespace(storageClient.ns).
		Resource(storagePluralName).
		VersionedParams(opts, scheme.ParameterCodec).
		Timeout(timeout).
		Watch(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to get watch interface for storage")
	}

	return watchInterface, nil
}

// Create takes the representation of a storage and creates it.
// Returns the server's representation of the storage, and an error, if there is any.
func (storageClient *StorageClient) Create(
	ctx context.Context,
	storage *ydbApi.Storage,
	opts *metav1.CreateOptions,
) (*ydbApi.Storage, error) {
	var err error

	result := &ydbApi.Storage{} //nolint
	if err = storageClient.client.Post().
		Namespace(storageClient.ns).
		Resource(storagePluralName).
		VersionedParams(opts, scheme.ParameterCodec).
		Body(storage).
		Do(ctx).
		Into(result); err != nil {
		return nil, errors.Wrap(err, "failed to create storage")
	}

	return result, nil
}

// Delete takes name of the storage and deletes it. Returns an error if one occurs.
func (storageClient *StorageClient) Delete(
	ctx context.Context,
	name string,
	opts *metav1.DeleteOptions,
) error {
	var err error

	if err = storageClient.client.Delete().
		Namespace(storageClient.ns).
		Resource(storagePluralName).
		Name(name).
		Body(&opts).
		Do(ctx).
		Error(); err != nil {
		return errors.Wrap(err, "failed to delete storage")
	}

	return nil
}

// Patch applies the patch and returns the patched storage.
func (storageClient *StorageClient) Patch( //nolint:dupl // api realization
	ctx context.Context,
	name string,
	patchType types.PatchType,
	data []byte,
	opts *metav1.PatchOptions,
	subresources ...string,
) (*ydbApi.Storage, error) {
	var (
		err    error
		result *ydbApi.Storage
	)

	if err = storageClient.client.Patch(patchType).
		Namespace(storageClient.ns).
		Resource(storagePluralName).
		Name(name).
		SubResource(subresources...).
		VersionedParams(opts, scheme.ParameterCodec).
		Body(data).
		Do(ctx).
		Into(result); err != nil {
		return nil, errors.Wrap(err, "failed to patch storage via rest")
	}

	return result, nil
}

// Apply takes the given apply declarative configuration,
// applies it and returns the applied ydb storage.
func (storageClient *StorageClient) Apply(
	ctx context.Context,
	storage *ydbApi.Storage,
	opts *metav1.ApplyOptions,
) (*ydbApi.Storage, error) {
	var (
		err  error
		data []byte
	)

	if storage == nil {
		return nil, errStorageMustNotBeNull
	}

	patchOpts := opts.ToPatchOptions()

	if data, err = json.Marshal(storage); err != nil {
		return nil, errors.Wrap(err, "failed to serialize storage into json")
	}

	if storage.Name == "" {
		return nil, errStorageNameMustNotBeNull
	}

	result := &ydbApi.Storage{} //nolint

	if err = storageClient.client.Patch(types.ApplyPatchType).
		Namespace(storageClient.ns).
		Resource(storagePluralName).
		Name(storage.Name).
		VersionedParams(&patchOpts, scheme.ParameterCodec).
		Body(data).
		Do(ctx).
		Into(result); err != nil {
		return nil, errors.Wrap(err, "failed to patch storage via rest")
	}

	return result, nil
}

type DatabaseInterface interface {
	Get(context.Context, string, *metav1.GetOptions) (*ydbApi.Database, error)
	List(context.Context, *metav1.ListOptions) (*ydbApi.DatabaseList, error)
	Watch(context.Context, *metav1.ListOptions) (watch.Interface, error)
	Create(context.Context, *ydbApi.Database, *metav1.CreateOptions) (*ydbApi.Database, error)
	Delete(context.Context, string, *metav1.DeleteOptions) error
	Patch(
		context.Context,
		string,
		types.PatchType,
		[]byte,
		*metav1.PatchOptions,
		...string,
	) (*ydbApi.Database, error)
	Apply(context.Context, *ydbApi.Database, *metav1.ApplyOptions) (*ydbApi.Database, error)
}

type DatabaseClient struct {
	client rest.Interface
	ns     string
}

func (ydbClient *YDBV1Alpha1Client) Databases(namespace string) DatabaseInterface {
	return &DatabaseClient{
		client: ydbClient.client,
		ns:     namespace,
	}
}

var (
	errDatabaseMustNotBeNull     = errors.New("storage provided to Apply must not be nil")
	errDatabaseNameMustNotBeNull = errors.New("storage.Name must be provided to Apply")
)

// Get takes name of the database, and returns the corresponding database object,
// and an error if there is any.
func (databaseClient *DatabaseClient) Get(
	ctx context.Context,
	name string,
	options *metav1.GetOptions,
) (*ydbApi.Database, error) {
	var err error
	result := &ydbApi.Database{} //nolint

	if err = databaseClient.client.Get().
		Namespace(databaseClient.ns).
		Resource(databasePluralName).
		Name(name).
		VersionedParams(options, scheme.ParameterCodec).
		Do(ctx).
		Into(result); err != nil {
		return nil, errors.Wrap(err, "failed to get database")
	}

	return result, nil
}

// List takes label and field selectors,
// and returns the list of Databases that match those selectors.
func (databaseClient *DatabaseClient) List(
	ctx context.Context,
	opts *metav1.ListOptions,
) (*ydbApi.DatabaseList, error) {
	var (
		err     error
		timeout time.Duration
	)

	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}

	result := &ydbApi.DatabaseList{} //nolint

	if err = databaseClient.client.Get().
		Namespace(databaseClient.ns).
		Resource(databasePluralName).
		VersionedParams(opts, scheme.ParameterCodec).
		Timeout(timeout).
		Do(ctx).
		Into(result); err != nil {
		return nil, errors.Wrap(err, "failed to get list of databases")
	}

	return result, nil
}

// Watch returns a watch.Interface that watches the requested Databases.
func (databaseClient *DatabaseClient) Watch(
	ctx context.Context,
	opts *metav1.ListOptions,
) (watch.Interface, error) {
	var (
		err            error
		timeout        time.Duration
		watchInterface watch.Interface
	)

	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}

	opts.Watch = true

	if watchInterface, err = databaseClient.client.Get().
		Namespace(databaseClient.ns).
		Resource(databasePluralName).
		VersionedParams(opts, scheme.ParameterCodec).
		Timeout(timeout).
		Watch(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to get watch interface for database")
	}

	return watchInterface, nil
}

// Create takes the representation of a database and creates it.
// Returns the server's representation of the database, and an error, if there is any.
func (databaseClient *DatabaseClient) Create(
	ctx context.Context,
	database *ydbApi.Database,
	opts *metav1.CreateOptions,
) (*ydbApi.Database, error) {
	var err error

	result := &ydbApi.Database{} //nolint
	if err = databaseClient.client.Post().
		Namespace(databaseClient.ns).
		Resource(databasePluralName).
		VersionedParams(opts, scheme.ParameterCodec).
		Body(database).
		Do(ctx).
		Into(result); err != nil {
		return nil, errors.Wrap(err, "failed to create database")
	}

	return result, nil
}

// Delete takes name of the database and deletes it. Returns an error if one occurs.
func (databaseClient *DatabaseClient) Delete(
	ctx context.Context,
	name string,
	opts *metav1.DeleteOptions,
) error {
	var err error

	if err = databaseClient.client.Delete().
		Namespace(databaseClient.ns).
		Resource(databasePluralName).
		Name(name).
		Body(&opts).
		Do(ctx).
		Error(); err != nil {
		return errors.Wrap(err, "failed to delete database")
	}

	return nil
}

// Patch applies the patch and returns the patched database.
func (databaseClient *DatabaseClient) Patch( //nolint:dupl // api realization
	ctx context.Context,
	name string,
	patchType types.PatchType,
	data []byte,
	opts *metav1.PatchOptions,
	subresources ...string,
) (*ydbApi.Database, error) {
	var (
		err    error
		result *ydbApi.Database
	)

	if err = databaseClient.client.Patch(patchType).
		Namespace(databaseClient.ns).
		Resource(databasePluralName).
		Name(name).
		SubResource(subresources...).
		VersionedParams(opts, scheme.ParameterCodec).
		Body(data).
		Do(ctx).
		Into(result); err != nil {
		return nil, errors.Wrap(err, "failed to patch database via rest")
	}

	return result, nil
}

// Apply takes the given apply declarative configuration,
// applies it and returns the applied ydb database.
func (databaseClient *DatabaseClient) Apply(
	ctx context.Context,
	database *ydbApi.Database,
	opts *metav1.ApplyOptions,
) (*ydbApi.Database, error) {
	var (
		err  error
		data []byte
	)

	if database == nil {
		return nil, errDatabaseMustNotBeNull
	}

	patchOpts := opts.ToPatchOptions()

	if data, err = json.Marshal(database); err != nil {
		return nil, errors.Wrap(err, "failed to serialize database into json")
	}

	if database.Name == "" {
		return nil, errDatabaseNameMustNotBeNull
	}

	result := &ydbApi.Database{} //nolint

	if err = databaseClient.client.Patch(types.ApplyPatchType).
		Namespace(databaseClient.ns).
		Resource(databasePluralName).
		Name(database.Name).
		VersionedParams(&patchOpts, scheme.ParameterCodec).
		Body(data).
		Do(ctx).
		Into(result); err != nil {
		return nil, errors.Wrap(err, "failed to patch database via rest")
	}

	return result, nil
}
