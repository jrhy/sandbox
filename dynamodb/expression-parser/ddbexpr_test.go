package ddbexpr

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type TestCase struct {
	desc     string
	expr     string
	ean      map[string]string
	eav      map[string]*dynamodb.AttributeValue
	item     interface{}
	expected bool
}

type Product struct {
	ProductDotReview Reviews `dynamodbav:"Product.Reviews"`
	Pictures         Pictures
}

type Reviews struct {
	OneStar []Review
}

type Review struct {
	Author, Review string
}

type (
	URL      string
	Pictures struct {
		FrontView URL
	}
)

func TestNumberParse(t *testing.T) {
	var f *float64

	e := Expression(`1`)
	f = e.parseNumber()
	require.NotNil(t, f)
	require.Equal(t, 1.0, *f)

	e = Expression(`1.1`)
	f = e.parseNumber()
	require.NotNil(t, f)
	require.Equal(t, 1.1, *f)

	e = Expression(`.1`)
	f = e.parseNumber()
	require.NotNil(t, f)
	require.Equal(t, 0.1, *f)

	e = Expression(`"`)
	f = e.parseNumber()
	require.Nil(t, f)
}

func TestStringLiteralParse(t *testing.T) {
	var s string
	e := Expression(`"foo"`)
	ok := e.parseStringLiteral(&s)
	assert.True(t, ok)
	assert.Equal(t, "foo", s)

	e = Expression(`""`)
	ok = e.parseStringLiteral(&s)
	assert.True(t, ok)
	assert.Equal(t, "", s)

	e = Expression(`"`)
	ok = e.parseStringLiteral(&s)
	assert.False(t, ok)
}

func TestEvaluateCondition(t *testing.T) {
	for _, tc := range []TestCase{
		{
			"attribute_type with SS",
			"attribute_type(Color, :v_sub)",
			nil,
			map[string]*dynamodb.AttributeValue{
				":v_sub": {S: aws.String("SS")},
			},
			struct {
				ID    int32
				Color []string `dynamodbav:",stringset"`
			}{456, []string{"sea foam green"}},
			true,
		},
		{
			"attribute_not_exists false when present",
			"attribute_not_exists(ID)",
			nil,
			nil,
			struct {
				ID int32
			}{456},
			false,
		},
		{
			"attribute_not_exists when not present",
			"attribute_not_exists(ID)",
			nil,
			nil,
			struct {
				ID int32 `dynamodbav:",omitempty"`
			}{0},
			true,
		},
		{
			"attribute_not_exists when NULL",
			"attribute_not_exists(ID)",
			nil,
			nil,
			struct {
				ID *int32
			}{nil},
			false,
		},
		{
			"attribute_exists",
			"attribute_exists(#pr.OneStar)",
			map[string]string{"#pr": "Product.Reviews"},
			nil,
			Product{
				ProductDotReview: Reviews{
					OneStar: []Review{{
						"me", "it was great",
					}},
				},
			},
			true,
		},
		{
			"attribute_exists false when not present",
			"attribute_exists(Goo)",
			nil,
			nil,
			struct {
				ID int32
			}{5},
			false,
		},
		{
			"string starting value example",
			"begins_with(Pictures.FrontView, :v_sub)",
			nil,
			map[string]*dynamodb.AttributeValue{
				":v_sub": {S: aws.String("http://")},
			},
			Product{Pictures: Pictures{FrontView: "http://foo"}},
			true,
		},
		{
			"string starting value example negative",
			"begins_with(Pictures.FrontView, :v_sub)",
			nil,
			map[string]*dynamodb.AttributeValue{
				":v_sub": {S: aws.String("http://")},
			},
			Product{Pictures: Pictures{FrontView: "xhttp://foo"}},
			false,
		},
		{
			"byte string prefix",
			"begins_with(Key, :v_sub)",
			nil,
			map[string]*dynamodb.AttributeValue{
				":v_sub": {B: []byte{1, 2, 3, 4, 5}},
			},
			struct{ Key []byte }{Key: []byte{1, 2, 3, 4, 5, 6}},
			true,
		},
		{
			"byte string prefix negative",
			"begins_with(Key, :v_sub)",
			nil,
			map[string]*dynamodb.AttributeValue{
				":v_sub": {B: []byte{1, 2, 3, 4, 5}},
			},
			struct{ Key []byte }{Key: []byte{1}},
			false,
		},
		{
			"byte string prefix wrong type",
			"begins_with(Key, :v_sub)",
			nil,
			map[string]*dynamodb.AttributeValue{
				":v_sub": {B: []byte{1, 2, 3, 4, 5}},
			},
			struct{ Key string }{Key: "hi"},
			false,
		},
		{
			"number comparison",
			"1 < 2",
			nil,
			nil,
			nil,
			true,
		}, /*{
			"attribute_type with different case, dunno what this should do",
			"attribute_type(color, :v_sub)",
			nil,
			map[string]*dynamodb.AttributeValue{":v_sub": {S: aws.String("SS")}},
			struct {
				ID    int32
				Color []string `dynamodbav:",stringset"`
			}{456, []string{"sea foam green"}},
			false,
		},*/{
			"parse path with EAV should fail",
			"attribute_type(Color.:v_sub, :v_sub)",
			nil,
			map[string]*dynamodb.AttributeValue{
				":v_sub": {S: aws.String("SS")},
			},
			struct {
				ID    int32
				Color []string `dynamodbav:",stringset"`
			}{456, []string{"sea foam green"}},
			false,
		},
		{
			"and",
			`1 < 2 or 3 < 4 and 5 > -1`,
			nil,
			nil,
			nil,
			true,
		},
		{
			"parenthesis",
			`(1 < 2 or 3 < 4) and not 5 < 0`,
			nil,
			nil,
			nil,
			true,
		},
		{
			"not",
			"not not not 5 < -1",
			nil,
			nil,
			nil,
			true,
		},
		{
			"size of string list entry",
			"size(#pr.OneStar[0].Review) > 4",
			map[string]string{"#pr": "Product.Reviews"},
			nil,
			Product{
				ProductDotReview: Reviews{
					OneStar: []Review{{
						"me", "it was great",
					}},
				},
			},
			true,
		},
		{
			"size of list",
			"size(#pr.OneStar) == 1",
			map[string]string{"#pr": "Product.Reviews"},
			nil,
			Product{
				ProductDotReview: Reviews{
					OneStar: []Review{{
						"me", "it was great",
					}},
				},
			},
			true,
		},
		{
			"between",
			"5 between 2 and 8",
			nil,
			nil,
			nil,
			true,
		},
		{
			"between negative",
			`not "e" between "b" and "c"`,
			nil,
			nil,
			nil,
			true,
		},
		{
			"in",
			`5 in (4, 5, 6)`,
			nil,
			nil,
			nil,
			true,
		},
		{
			"contains",
			`contains(Color, :v_sub)`,
			nil,
			map[string]*dynamodb.AttributeValue{
				":v_sub": {S: aws.String("Red")},
			},
			struct {
				Color []string `dynamodbav:",stringset"`
			}{Color: []string{"Yellow", "Red"}},
			true,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			actual := EvaluateCondition(tc.expr, tc.ean, tc.eav, tc.item)
			require.Equal(t, tc.expected, actual, tc.desc)
		})
	}
}
