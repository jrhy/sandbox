OUTPUT=output

run: ${OUTPUT}/rust_from_go
	${OUTPUT}/rust_from_go

${OUTPUT}/rust_from_go: ${OUTPUT} ${OUTPUT}/librfg.a main.go
	CGO_ENABLED=1 go build -o ${OUTPUT}/rust_from_go

${OUTPUT}/librfg.a: rfg_int.rs
	rustc --crate-type staticlib -o ${OUTPUT}/librfg.a rfg_int.rs

${OUTPUT}:
	-mkdir ${OUTPUT}

clean:
	-rm -fr ${OUTPUT}

