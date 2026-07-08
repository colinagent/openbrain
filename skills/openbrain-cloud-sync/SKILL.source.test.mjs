import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const skill = readFileSync(new URL('./SKILL.md', import.meta.url), 'utf8');

test('Cloud Sync skill requires structured decision questions', () => {
  assert.match(skill, /`questions\[\]` options/);
  assert.match(skill, /questions\[\]\.options\[\]/);
  assert.match(skill, /`questions\[\]\.options\[\]\.id\/label`/);
  assert.match(skill, /Do not add per-question headers, `allowOther`,\s+option descriptions, or option tones/);
  assert.match(skill, /Body-only option\s+lists are invalid/);
  assert.match(skill, /Other\.\.\./);
  assert.match(skill, /message_update/);
  assert.doesNotMatch(skill, /"header"/);
  assert.doesNotMatch(skill, /"allowOther"/);
  assert.doesNotMatch(skill, /"description"/);
  assert.doesNotMatch(skill, /"tone"/);
});

test('Cloud Sync skill handles user answer replies through workspace AGENTS.md', () => {
  assert.match(skill, /selectedSkillContext\.messageSystem=true/);
  assert.match(skill, /requestTitle/);
  assert.match(skill, /requestTitle` is the business\s+anchor/);
  assert.match(skill, /questionID` and `optionID/);
  assert.match(skill, /answers/);
  assert.match(skill, /optionID/);
  assert.match(skill, /other:true/);
  assert.match(skill, /--allow-nested/);
  assert.match(skill, /OpenBrain Cloud Sync Decisions/);
  assert.doesNotMatch(skill, /legacy/i);
  assert.match(skill, /<workspacePath>\/AGENTS\.md/);
  assert.match(skill, /Do not create `AGENTS\.md`/);
});

test('Cloud Sync skill documents nested git option ids', () => {
  for (const optionID of [
    'convert-submodule',
    'keep-independent',
    'vendor-regular-files',
    'remove-nested-repo',
  ]) {
    assert.match(skill, new RegExp(`\`${optionID}\``));
  }
});
