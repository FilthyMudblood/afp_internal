.PHONY: all clean tectonic

all: paper.pdf

paper.pdf: paper.tex references.bib neurips_2026.sty
	pdflatex -interaction=nonstopmode paper
	bibtex paper
	pdflatex -interaction=nonstopmode paper
	pdflatex -interaction=nonstopmode paper

tectonic: paper.tex references.bib neurips_2026.sty
	tectonic -X compile paper.tex

clean:
	rm -f paper.aux paper.bbl paper.blg paper.log paper.out paper.pdf
