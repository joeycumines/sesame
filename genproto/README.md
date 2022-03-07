# Go generated proto packages

This is a top-level `genproto` dir structure for packages under the current sesame API version that don't have a better
place for their Go package. Generally, a "better place" means there's other packages which make sense to keep as close
as possible.

The actual packages should be at least moderately predictable, based on the path to their `*.proto` files in
[../schema](../schema). The general style is based on the
[google.golang.org/genproto](https://github.com/googleapis/go-genproto) Go module.
