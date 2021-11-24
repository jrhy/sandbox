fn main() {
    let path = "./target";
    let lib = "gob2json";

    println!("cargo:rustc-link-search=native={}", path);
    println!("cargo:rustc-link-lib=static={}", lib);
}
