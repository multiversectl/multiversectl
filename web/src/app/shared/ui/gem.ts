import { booleanAttribute, ChangeDetectionStrategy, Component, computed, inject, input } from '@angular/core';
import { DomSanitizer } from '@angular/platform-browser';

// The gems, as the herdr plugin draws them (multiversectl/herdr-plugin,
// tools/gems.mjs): a repository is a stone whose colour comes from its name, so
// it is the same in every universe and in herdr; a universe, made of
// repositories, is the ring of all six.
const GEMS: Record<string, [string, string, string]> = {
  red: ['#ff4d4d', '#ffc2b8', '#7a0812'],
  blue: ['#4d86ff', '#c2d6ff', '#0b237a'],
  orange: ['#ff9433', '#ffd9b0', '#7a3300'],
  yellow: ['#ffd633', '#fff5bf', '#7a5c00'],
  purple: ['#b85cff', '#ecd1ff', '#43087a'],
  green: ['#45d957', '#c9ffc0', '#0a5c17'],
};
const ORDER = ['red', 'blue', 'yellow', 'green', 'purple', 'orange'];
const RIM = [[12, 2.8], [16.6, 4.6], [19, 9.4], [18.2, 15.2], [14, 20.4], [9.2, 20], [5.6, 15.8], [5, 9.6], [7.6, 4.8]];
const TABLE = [[12, 7.4], [15, 9], [15.4, 12.8], [12.8, 15.8], [9.6, 15.2], [8.4, 11.6], [9.4, 8.4]];

/** The colour of a repository's gem: FNV-1a of its name, as herdr's panel picks it. */
export function gemOf(name: string): string {
  let h = 0x811c9dc5;
  for (const ch of new TextEncoder().encode(name)) h = Math.imul(h ^ ch, 0x01000193) >>> 0;
  return ORDER[h % ORDER.length];
}

const f = (n: number) => n.toFixed(2);
function stone(colour: string, id: string, s = 1, dx = 0, dy = 0): string {
  const [base, light, dark] = GEMS[colour];
  const pts = (p: number[][]) => p.map(([x, y]) => `${f(x * s + dx)},${f(y * s + dy)}`).join(' ');
  const cx = 12 * s + dx, cy = 11.6 * s + dy;
  const facets = RIM.map(([x, y], i) => {
    const [tx, ty] = TABLE[Math.round((i * TABLE.length) / RIM.length) % TABLE.length];
    return `M${f(x * s + dx)} ${f(y * s + dy)}L${f(tx * s + dx)} ${f(ty * s + dy)}`;
  }).join('');
  return `<defs><radialGradient id="${id}g" cx="${cx}" cy="${cy}" r="${11.5 * s}" gradientUnits="userSpaceOnUse"><stop offset="0" stop-color="${base}" stop-opacity=".55"/><stop offset="1" stop-color="${base}" stop-opacity="0"/></radialGradient>`
    + `<radialGradient id="${id}s" cx="${cx - s}" cy="${cy - 1.2 * s}" r="${9 * s}" gradientUnits="userSpaceOnUse"><stop offset="0" stop-color="${light}"/><stop offset=".45" stop-color="${base}"/><stop offset="1" stop-color="${dark}"/></radialGradient></defs>`
    + `<circle cx="${cx}" cy="${cy}" r="${11.5 * s}" fill="url(#${id}g)"/>`
    + `<polygon points="${pts(RIM)}" fill="url(#${id}s)" stroke="${light}" stroke-opacity=".7" stroke-width="${0.6 * s}" stroke-linejoin="round"/>`
    + `<polygon points="${pts(TABLE)}" fill="${light}" fill-opacity=".18" stroke="#fff" stroke-opacity=".35" stroke-width="${0.45 * s}" stroke-linejoin="round"/>`
    + `<path d="${facets}" stroke="#fff" stroke-opacity=".28" stroke-width="${0.4 * s}"/>`
    + `<path d="M${cx - 0.6 * s} ${cy - s}l${0.5 * s} ${-2.6 * s}l${0.5 * s} ${2.6 * s}l${2.6 * s} ${0.5 * s}l${-2.6 * s} ${0.5 * s}l${-0.5 * s} ${2.6 * s}l${-0.5 * s} ${-2.6 * s}l${-2.6 * s} ${-0.5 * s}z" fill="#fff" fill-opacity=".9"/>`
    + `<circle cx="${cx - 0.1 * s}" cy="${cy - 0.5 * s}" r="${0.88 * s}" fill="#fff"/>`;
}
function ring(id: string): string {
  return ORDER.map((c, i) => {
    const a = -Math.PI / 2 + (i * Math.PI) / 3, s = 0.4;
    return stone(c, `${id}${i}`, s, 12 + 7.4 * Math.cos(a) - 12 * s, 12 + 7.4 * Math.sin(a) - 11.6 * s);
  }).join('');
}

let seq = 0;

/** A repository's gem (`of` its name), or a universe's ring of them (`ring`). */
@Component({
  selector: 'mv-gem',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: '',
  host: { '[innerHTML]': 'html()', '[class.dim]': 'dim()', 'aria-hidden': 'true' },
  styles: `:host { display: inline-flex; width: 1.15em; height: 1.15em; flex: none; vertical-align: -0.2em; transition: opacity .2s, filter .2s }
    :host.dim { opacity: .45; filter: saturate(.5) } :host ::ng-deep svg { width: 100%; height: 100%; overflow: visible }`,
})
export class Gem {
  /** The repository whose gem this is. */
  readonly of = input<string>('');
  /** The universe's ring instead. */
  readonly ring = input(false, { transform: booleanAttribute });
  /** Unlit: the universe is stopped. */
  readonly dim = input(false);

  private readonly id = `gem${++seq}-`;
  private readonly sanitizer = inject(DomSanitizer);
  // Drawn here from constants and a colour picked from a fixed list: nothing a
  // name can reach, so it is safe to skip the sanitizer that would strip the SVG.
  protected readonly html = computed(() => this.sanitizer.bypassSecurityTrustHtml(
    `<svg viewBox="1.5 0.5 21 22" fill="none">${this.ring() ? ring(this.id) : stone(gemOf(this.of()), this.id)}</svg>`));
}
