pub struct Circle {
    r: f64,
}

impl Circle {
    pub fn area(&self) -> f64 {
        3.14 * self.r * self.r
    }
}

impl Shape for Circle {
    fn area(&self) -> f64 {
        Circle::area(self)
    }
}

mod tests {
    fn it_works() {
        assert!(true);
    }
}
