//! A small library.

/* fn commented_out() {} */

/*
pub fn also_commented() -> i32 {
    1
}
*/

pub struct Grid<'a> {
    cells: &'a [u8],
}

impl<'a> Grid<'a> {
    pub fn new(cells: &'a [u8]) -> Self {
        Grid { cells }
    }

    pub fn len(&self) -> usize {
        self.cells.len()
    }
}

pub trait Shape {
    fn area(&self) -> f64;
}

impl Shape for Grid<'_> {
    fn area(&self) -> f64 {
        self.len() as f64
    }
}

const HELP: &str = r#"
fn not_a_function() {
struct NotAStruct;
"#;

pub fn help() -> &'static str {
    let raw = r"
impl NotAnImpl {
";
    let _ = raw;
    HELP
}

pub fn quotes() -> (char, char, &'static str) {
    ('"', '{', "a plain string
fn in_a_plain_string() {
")
}

const BYTES: &[u8] = br"
struct NotInBytes {
";

macro_rules! square {
    ($x:expr) => {
        $x * $x
    };
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn grid_len() {
        assert_eq!(Grid::new(&[1, 2]).len(), 2);
    }
}
