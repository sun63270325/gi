# Python wrapping of GoGi

You can now run most of GoGi via Python, using a newly-updated version of the [gopy](https://github.com/go-python/gopy) tool that automatically creates Python bindings for Go packages.  Until the pull-request is merged into go-python, there is an updated README at the [goki fork](https://github.com/goki/gopy) (note: you can no longer build directly from this goki fork -- see instructions below for merging into go-python).

Go incorporates many features found in Python, and provides a really natural "backend" language for computationally-intensive functionality such as a GUI.  Python provides a nice interactive "scripting" level interface for dynamically building GUI's, and can bring Go code to a much wider audience.  Thus, this represents an ideal combination of the two languages.  And you can build the entire stack, from the raw Go code in GoGi to the Python bindings (which unfortunately are a bit slow because they rely on a C compiler..), in a tiny fraction of the time it takes to build something like Qt and the PySide or PyQt bindings.

# Installation

*Note: Windows is completely untested and very unlikely to work* -- there is nothing in principle preventing it from working, but it just requires a bunch of special stuff and we haven't had a chance to get to it.

Python version 3 (3.6 has been well tested) is recommended, and the instructions assume that version (you can probably get version 2 to work but it has not been tested).  Also pip must be installed, as is typical.  This assumes you have already installed GoGi per the [Wiki Install](https://github.com/goki/gi/wiki/Install) instructions, including installing [Go itself](https://golang.org/doc/install), and adding `~/go/bin` to your `PATH`.  *be double-sure* that `goki/examples/widgets` runs properly per wiki install before proceeding -- if that doesn't work, nothing else will.

On linux, you must ensure that the linker `ld` will look in the current directory for library files -- add this to your `.bashrc` file (and `source` that file after editing, or enter command locally):

```sh
export LD_LIBRARY_PATH=$LD_LIBRARY_PATH:.
```

**NOTE:** as of 8/28/2019, these instructions *no longer* include extra steps to update gopy as it has been updated in the go-python repository.

```sh
$ python3 -m pip install --upgrade pybindgen setuptools wheel
$ go get golang.org/x/tools/cmd/goimports
$ go get github.com/go-python/gopy 
$ cd ~/go/src/github.com/go-python/gopy  # use $GOPATH instead of ~/go if somewhere else
$ go install    # do go get -u ./... if this fails and try again -- installs gopy exe in ~go/bin
$ cd ~/go/src/github.com/goki/gi/python   # again, $GOPATH etc..
$ make  # if you get an error about not finding gopy, make sure ~/go/bin is on your path
$ make install  # may need to do: sudo make install -- installs into /usr/local/bin and python site-packages
$ cd ../examples/widgets
$ pygi   # this was installed during make install into /usr/local/bin
$ import widgets  # this loads and runs widgets.py -- view that and compare with widgets.go
$ pygi -i widgets.py  # another way to start it
$ ./widgets.py        # yet another way to start it, using #! comment magic at start
```

If you get something like this error:
```sh
dyld: Library not loaded: @rpath/libpython3.6m.dylib
```
then you need to make sure that this lib is on your LD_LIBRARY_PATH -- on mac you can do `otool -L /usr/local/bin/pygi` and on linux it is `ldd /usr/local/bin/pygi` -- that should show you where it is trying to find that library.

If `pkg-config` or some other misc command is not found, you can use `brew install` to install it using homebrew on mac, or your package manager on linux.

# How it works

* `gopy` `exe` mode builds a standalone executable called `pygi` that combines the python interpreter and shell, with the GoGi Go libraries.  This is needed because the GUI event loop must run on the main thread, which otherwise is taken by the python interpreter if you try to load GoGi as a library into the standard python executable (gopy can also be loaded as a library for other cases where the thread conflict is not a problem).

* The entire gi codebase is available via stub functions in a `_gi` module.  There are various `.py` python wrappers into that `_gi` module corresponding to each of the different packages in GoGi, such as `gi`, `giv`, `units`, etc.  These are all installed during `make install` into a single python module called `gi`.

* As you can see in the `widgets.py` file, you load the individual packages as follows:

```Python
from gi import go, gi, giv, units, ki, gimain
```

* The `go` package is common to all `gopy` modules and is the home of all the standard go types and any other misc types outside the scope of the processed packages, that are referred to by those packages.  For example, `go.Slice_int` is a standard Go `[]int` slice.  `go.GoClass` is the base class of all class wrappers generated by gopy, so it can be used to determine if something is a go-based class or not:

```Python
if isinstance(ojb, go.GoClass):
```

* All non-basic types (e.g., anything that is not an `int` `float` `string`, such as a `slice` or `map` or `struct`) "live" in the Go world, and the python side uses a unique `handle` identifier (an `int64`) to refer to those Go objects.  Most of the time the handles are handled automatically by the python wrapper, but sometimes, you'll see code that initializes a new object using these handles, as in a callback function:

```Python
def strdlgcb(recv, send, sig, data):
    dlg = gi.Dialog(handle=send) # send is a raw int64 handle -- use it to initialize
    # a python wrapper class of type gi.Dialog -- note: you must get these types
    # right and crashes are likely if you don't!
```

* Unfortunately, Python does not allow callback functions to be class methods, if the thing calling the callback (i.g., GoGi) is running on a different thread, which it is.  Thus, you can only use separate standalone functions for callbacks.  All GoGi callbacks have that same signature of recv, send, sig, and data, and data is automatically converted to a string type because the native Go `interface{}` type is not really managable within Python.

* Also due to the issues with the `interface{}` type, the widely-used `ki.Props` class in Go, which is used for styling properties etc, must be used handled using `ki.SetPropStr` and `ki.SetSubProps` methods, etc, which set specific types of properties (strings, sub-Props properties).  In general, most style properties can be set using a string representation, so just use that wherever possible.

* A Python-based module called `pygiv` is under development, which will provide the `giv` View-based interfaces for native Python types, so that for example you can use a `pygiv.ClassView` to get an instant GUI editor of a Python `class` object, in the same way that `giv.StructView` provides a GUI editor of Go `struct`s.


