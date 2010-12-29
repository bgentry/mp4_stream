all: install

DIRS=\
     mp4\

%.install:
	+cd $* && gomake install

install: $(addsuffix .install, $(DIRS))

%.clean:
	+cd $* && gomake clean

clean: $(addsuffix .clean, $(DIRS))
