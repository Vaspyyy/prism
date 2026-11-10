// Object types in a signature, and values without a block or a semicolon.
function split(minutes: number): { amount: number; unit: string } {
  return { amount: minutes, unit: "minutes" };
}

const boot = async (name: string): Promise<{ sample?: Sample; failure?: string }> => {
  return {};
};

export function parse(
  line: string,
): { ok: boolean } | null {
  return null;
}

export class Store<T extends { id: string }> extends Base<T> {
  get(id: string): T | undefined {
    return undefined;
  }
}

export const abs = (n: bigint): bigint => (n < zero ? -n : n)

export const gcd: {
  (a: bigint): bigint
} = dual(2, euclid)

const platform = (c: Compression): Context<Platform> =>
  Context.make(
    Platform,
    { c },
  )

export type ToolError =
  | NotFound
  | Rejected

const next = () => count++

if (ready) {
  start()
}

const ring = (cx: number, color: string): Shape => ({
  color,
  compile: (grid) => {
    grid.set(cx);
  },
});

const models = (() => {
  const out = [];
  return out;
})();

export const mocks = {
  search: ((input: unknown) => {
    state.search(input);
  }) as never,
};

const add = (a: number) => ({ b }: { b: number }) =>
  a + b;
