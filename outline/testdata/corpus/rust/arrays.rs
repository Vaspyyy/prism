//! Array types: a ; in brackets isn't the end of a definition.

pub struct Hit {
    pos: [i32; 3],
}

impl Hit {
    fn beside(&self) -> [i32; 3] {
        [self.pos[0] + 1, self.pos[1], self.pos[2]]
    }

    pub fn nearest(
        &self,
        at: [f64; 3],
    ) -> Option<[i32; 3]> {
        None
    }
}

impl Codec<[i32; 3]> for Hit {
    fn read(buf: &mut Buf) -> [i32; 3] {
        [buf.int(), buf.int(), buf.int()]
    }
}

static CORNERS: [Hit; 2] = [
    Hit { pos: [0, 0, 0] },
    Hit { pos: [1, 1, 1] },
];

fn literals(&self) -> ArrayVec<Literal, { Size::MAX }> {
    ArrayVec::new()
}

type Hash = [u8; 32];
