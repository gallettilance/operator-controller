package resolution_test

import (
	"context"
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/operator-framework/deppy/pkg/deppy"
	"github.com/operator-framework/deppy/pkg/deppy/input"
	"github.com/operator-framework/deppy/pkg/deppy/solver"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/operator-framework/operator-controller/api/v1alpha1"
	"github.com/operator-framework/operator-controller/internal/resolution/variablesources"
)

func TestOperatorResolver(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Operator Resolver Suite")
}

func FakeClient(objects ...client.Object) client.Client {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		panic(fmt.Sprintf("error creating fake client: %s", err))
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
}

var testEntityCache = map[deppy.Identifier]input.Entity{
	"operatorhub/prometheus/0.37.0": *input.NewEntity("operatorhub/prometheus/0.37.0", map[string]string{
		"olm.bundle.path": `"quay.io/operatorhubio/prometheus@sha256:3e281e587de3d03011440685fc4fb782672beab044c1ebadc42788ce05a21c35"`,
		"olm.channel":     "{\"channelName\":\"beta\",\"priority\":0,\"replaces\":\"prometheusoperator.0.32.0\"}",
		"olm.gvk":         "[{\"group\":\"monitoring.coreos.com\",\"kind\":\"Alertmanager\",\"version\":\"v1\"}, {\"group\":\"monitoring.coreos.com\",\"kind\":\"Prometheus\",\"version\":\"v1\"}]",
		"olm.package":     "{\"packageName\":\"prometheus\",\"version\":\"0.37.0\"}",
	}),
	"operatorhub/prometheus/0.47.0": *input.NewEntity("operatorhub/prometheus/0.47.0", map[string]string{
		"olm.bundle.path": `"quay.io/operatorhubio/prometheus@sha256:5b04c49d8d3eff6a338b56ec90bdf491d501fe301c9cdfb740e5bff6769a21ed"`,
		"olm.channel":     "{\"channelName\":\"beta\",\"priority\":0,\"replaces\":\"prometheusoperator.0.37.0\"}",
		"olm.gvk":         "[{\"group\":\"monitoring.coreos.com\",\"kind\":\"Alertmanager\",\"version\":\"v1\"}, {\"group\":\"monitoring.coreos.com\",\"kind\":\"Prometheus\",\"version\":\"v1alpha1\"}]",
		"olm.package":     "{\"packageName\":\"prometheus\",\"version\":\"0.47.0\"}",
	}),
	"operatorhub/packageA/2.0.0": *input.NewEntity("operatorhub/packageA/2.0.0", map[string]string{
		"olm.bundle.path": `"foo.io/packageA/packageA:v2.0.0"`,
		"olm.channel":     "{\"channelName\":\"stable\",\"priority\":0}",
		"olm.gvk":         "[{\"group\":\"foo.io\",\"kind\":\"Foo\",\"version\":\"v1\"}]",
		"olm.package":     "{\"packageName\":\"packageA\",\"version\":\"2.0.0\"}",
	}),
}

var _ = Describe("OperatorResolver", func() {
	It("should resolve the packages described by the available Operator resources", func() {
		resources := []client.Object{
			&v1alpha1.Operator{
				ObjectMeta: metav1.ObjectMeta{
					Name: "prometheus",
				},
				Spec: v1alpha1.OperatorSpec{
					PackageName: "prometheus",
				},
			},
			&v1alpha1.Operator{
				ObjectMeta: metav1.ObjectMeta{
					Name: "packageA",
				},
				Spec: v1alpha1.OperatorSpec{
					PackageName: "packageA",
				},
			},
		}
		client := FakeClient(resources...)
		entitySource := input.NewCacheQuerier(testEntityCache)
		variableSource := variablesources.NewOperatorVariableSource(client)
		resolver := solver.NewDeppySolver(entitySource, variableSource)
		solution, err := resolver.Solve(context.Background())
		Expect(err).ToNot(HaveOccurred())
		// 2 * required package variables + 2 * bundle variables
		Expect(solution.SelectedVariables()).To(HaveLen(4))

		Expect(solution.IsSelected("operatorhub/packageA/2.0.0")).To(BeTrue())
		Expect(solution.IsSelected("operatorhub/prometheus/0.47.0")).To(BeTrue())
		Expect(solution.IsSelected("required package packageA")).To(BeTrue())
		Expect(solution.IsSelected("required package prometheus")).To(BeTrue())

		Expect(solution.IsSelected("operatorhub/prometheus/0.37.0")).To(BeFalse())

	})

	It("should not return an error if there are no Operator resources", func() {
		var resources []client.Object
		client := FakeClient(resources...)
		entitySource := input.NewCacheQuerier(testEntityCache)
		variableSource := variablesources.NewOperatorVariableSource(client)
		resolver := solver.NewDeppySolver(entitySource, variableSource)
		solution, err := resolver.Solve(context.Background())
		Expect(err).ToNot(HaveOccurred())
		Expect(solution.SelectedVariables()).To(BeEmpty())
	})

	It("should return an error if the entity source throws an error", func() {
		resource := &v1alpha1.Operator{
			ObjectMeta: metav1.ObjectMeta{
				Name: "prometheus",
			},
			Spec: v1alpha1.OperatorSpec{
				PackageName: "prometheus",
			},
		}
		client := FakeClient(resource)
		entitySource := FailEntitySource{}
		variableSource := variablesources.NewOperatorVariableSource(client)
		resolver := solver.NewDeppySolver(entitySource, variableSource)
		solution, err := resolver.Solve(context.Background())
		Expect(solution).To(BeNil())
		Expect(err).To(HaveOccurred())
	})

	It("should return an error if the client throws an error", func() {
		client := NewFailClientWithError(fmt.Errorf("something bad happened"))
		entitySource := input.NewCacheQuerier(testEntityCache)
		variableSource := variablesources.NewOperatorVariableSource(client)
		resolver := solver.NewDeppySolver(entitySource, variableSource)
		solution, err := resolver.Solve(context.Background())
		Expect(solution).To(BeNil())
		Expect(err).To(HaveOccurred())
	})
})

var _ input.EntitySource = &FailEntitySource{}

type FailEntitySource struct{}

func (f FailEntitySource) Get(_ context.Context, _ deppy.Identifier) (*input.Entity, error) {
	return nil, fmt.Errorf("error calling get in entity source")
}

func (f FailEntitySource) Filter(_ context.Context, _ input.Predicate) (input.EntityList, error) {
	return nil, fmt.Errorf("error calling filter in entity source")
}

func (f FailEntitySource) GroupBy(_ context.Context, _ input.GroupByFunction) (input.EntityListMap, error) {
	return nil, fmt.Errorf("error calling group by in entity source")
}

func (f FailEntitySource) Iterate(_ context.Context, _ input.IteratorFunction) error {
	return fmt.Errorf("error calling iterate in entity source")
}

var _ client.Client = &FailClient{}

type FailClient struct {
	client.Client
	err error
}

func NewFailClientWithError(err error) client.Client {
	return &FailClient{
		Client: FakeClient(),
		err:    err,
	}
}

func (f FailClient) Get(_ context.Context, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
	return f.err
}

func (f FailClient) List(_ context.Context, _ client.ObjectList, _ ...client.ListOption) error {
	return f.err
}

func (f FailClient) Create(_ context.Context, _ client.Object, _ ...client.CreateOption) error {
	return f.err
}

func (f FailClient) Delete(_ context.Context, _ client.Object, _ ...client.DeleteOption) error {
	return f.err
}

func (f FailClient) Update(_ context.Context, _ client.Object, _ ...client.UpdateOption) error {
	return f.err
}

func (f FailClient) Patch(_ context.Context, _ client.Object, _ client.Patch, _ ...client.PatchOption) error {
	return f.err
}

func (f FailClient) DeleteAllOf(_ context.Context, _ client.Object, _ ...client.DeleteAllOfOption) error {
	return f.err
}
