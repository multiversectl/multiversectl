import { booleanAttribute, ChangeDetectionStrategy, Component, computed, input } from '@angular/core';

// The gems, drawn by web/tools/gems.mjs into public/gems (herdr's Universes
// panel shows the same files). A repository's colour is its place among its
// multiverse's repositories, sorted, so it is the same in every universe and
// in herdr; a universe is the ring of its repositories' gems.
export const GEMS = ['red', 'blue', 'yellow', 'green', 'purple', 'cyan'] as const;

/** The gem colour of repo among a multiverse's repos; '' when it is not one of them. */
export function gemOf(repos: readonly string[], repo: string): string {
  const i = [...repos].sort().indexOf(repo);
  return i < 0 ? '' : GEMS[i % GEMS.length];
}

/** One gem: the plain small cut, or the faceted, glowing one (`large`, from 20px). */
@Component({
  selector: 'mv-gem',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `<img [src]="src()" alt="" draggable="false" />`,
  styles: `:host { display: inline-flex; width: 1.15em; height: 1.15em; flex: none; vertical-align: -0.2em } img { width: 100%; height: 100% }`,
})
export class Gem {
  readonly colour = input.required<string>();
  readonly large = input(false, { transform: booleanAttribute });
  protected readonly src = computed(() => `gems/${this.colour()}${this.large() ? '' : '-sm'}.svg`);
}

/** A universe: its repositories' gems in a ring, dim when it is stopped. */
@Component({
  selector: 'mv-ring',
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: { '[class.dim]': 'dim()', 'aria-hidden': 'true' },
  template: `@for (s of stones(); track $index) {
    <img [src]="'gems/' + s.colour + '-sm.svg'" alt="" draggable="false" [style.width.%]="s.size" [style.left.%]="s.x" [style.top.%]="s.y" />
  }`,
  styles: `:host { position: relative; display: inline-block; width: 1.25em; height: 1.25em; flex: none; vertical-align: -0.2em; transition: opacity .2s, filter .2s }
    :host.dim { opacity: .45; filter: saturate(.4) } img { position: absolute; height: auto }`,
})
export class Ring {
  /** The colour of each of the universe's repositories. */
  readonly colours = input.required<readonly string[]>();
  readonly dim = input(false, { transform: booleanAttribute });
  protected readonly stones = computed(() => {
    const cs = this.colours().filter(Boolean);
    const n = cs.length;
    if (n === 1) return [{ colour: cs[0], size: 80, x: 10, y: 10 }];
    // Stones shrink as there are more; a pair sits side by side.
    const size = n === 2 ? 52 : n <= 4 ? 46 : 40, r = (100 - size) / 2, start = n === 2 ? Math.PI : -Math.PI / 2;
    return cs.map((colour, i) => {
      const a = start + (i * 2 * Math.PI) / n;
      return { colour, size, x: 50 + r * Math.cos(a) - size / 2, y: 50 + r * Math.sin(a) - size / 2 };
    });
  });
}
