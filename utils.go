package main

import (
	"errors"
	"slices"
	"strings"
	"sync"

	"github.com/JoshPattman/jpf"
)

// ParMapDo runs the function for each item in inputs in parallel, returning an error if any occurred.
func ParMapDo[T any](inputs []T, fn func(T) error) error {
	_, err := ParMap(inputs, func(input T) (struct{}, error) {
		return struct{}{}, fn(input)
	})
	return err
}

// ParMapRange runs fn for every integer from 0 to upTo-1 in parallel, returning the results or an error if any occurred.
func ParMapRange[U any](upTo int, fn func(int) (U, error)) ([]U, error) {
	inputs := make([]int, upTo)
	for i := 0; i < upTo; i++ {
		inputs[i] = i
	}
	return ParMap(inputs, fn)
}

// Run every input through fn in parallel, returning the results or an error if any occurred.
func ParMap[T, U any](inputs []T, fn func(T) (U, error)) ([]U, error) {
	results := make([]U, len(inputs))
	errs := make([]error, len(inputs))
	wg := &sync.WaitGroup{}
	wg.Add(len(inputs))
	for i, input := range inputs {
		go func(i int, input T) {
			defer wg.Done()
			result, err := fn(input)
			if err != nil {
				errs[i] = err
				return
			}
			results[i] = result
		}(i, input)
	}
	wg.Wait()
	errs = slices.DeleteFunc(errs, func(err error) bool { return err == nil })
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return results, nil
}

func wrapJsonDecoder[T, U any](dec jpf.ResponseDecoder[T, U]) jpf.ResponseDecoder[T, U] {
	return jpf.NewSubstringResponseDecoder(
		dec,
		func(s string) (string, error) {
			startIndex := strings.Index(s, "{")
			endIndex := strings.LastIndex(s, "}")
			if startIndex == -1 {
				startIndex = 0
			}
			if endIndex == -1 || endIndex <= startIndex {
				endIndex = len(s) - 1
			}
			return s[startIndex : endIndex+1], nil
		},
	)
}
