import assert from 'node:assert/strict';
import test from 'node:test';
import {
  buildOpenBrainLogoGradientStops,
  normalizeToHex,
  OPENBRAIN_LOGO_GRADIENT_STOPS,
} from './openBrainLogoGradient.ts';

test('normalizeToHex converts rgb to hex', () => {
  assert.equal(normalizeToHex('rgb(244, 249, 247)'), '#F4F9F7');
  assert.equal(normalizeToHex('#f4f9f7'), '#F4F9F7');
});

test('buildOpenBrainLogoGradientStops preserves reference endpoints', () => {
  const light = buildOpenBrainLogoGradientStops('#FFFFFF', 'lighten');
  const dark = buildOpenBrainLogoGradientStops('#000000', 'darken');

  assert.deepEqual(light, OPENBRAIN_LOGO_GRADIENT_STOPS.light);
  assert.deepEqual(dark, OPENBRAIN_LOGO_GRADIENT_STOPS.dark);
});

test('buildOpenBrainLogoGradientStops adapts tail to theme background', () => {
  const light = buildOpenBrainLogoGradientStops('#F4F9F7', 'lighten');
  const dark = buildOpenBrainLogoGradientStops('#101816', 'darken');

  assert.equal(light.at(-1)?.color.toUpperCase(), '#F4F9F7');
  assert.equal(dark.at(-1)?.color.toUpperCase(), '#101816');
  assert.notEqual(light.at(-2)?.color, OPENBRAIN_LOGO_GRADIENT_STOPS.light.at(-2)?.color);
  assert.notEqual(dark.at(-2)?.color, OPENBRAIN_LOGO_GRADIENT_STOPS.dark.at(-2)?.color);
});
