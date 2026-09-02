// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This build always uses the portable implementation: the assembly variants
// shipped with golang.org/x/crypto are not vendored here.

package poly1305

type mac struct{ macGeneric }
