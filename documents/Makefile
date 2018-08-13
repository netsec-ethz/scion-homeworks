.PHONY: all clean
UNOCONV := $(shell command -v unoconv &> /dev/null)
DOCS = Questionnaire.pdf LatencyHomework.pdf 
SRC_FILES := $(wildcard *.odt)
PDF_FILES := $(patsubst %.odt,%.pdf,$(SRC_FILES))

all: unoconv_installed $(PDF_FILES)

%.pdf: %.odt
	unoconv -f pdf $^

clean:
	rm -f *.pdf

unoconv_installed:
ifndef UNOCONV
	sudo apt-get install -y unoconv
endif
