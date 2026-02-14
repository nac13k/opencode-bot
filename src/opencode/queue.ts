export class KeyedQueue {
  private readonly chains = new Map<string, Promise<void>>();

  async run<T>(key: string, operation: () => Promise<T>): Promise<T> {
    const previous = this.chains.get(key) ?? Promise.resolve();
    let release: () => void = () => undefined;
    const next = new Promise<void>((resolve) => {
      release = resolve;
    });
    this.chains.set(key, previous.then(() => next));
    await previous;

    try {
      return await operation();
    } finally {
      release();
    }
  }
}
