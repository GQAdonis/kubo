import { fromDagJson } from '../dagjson.js';
import type { Executor } from '../types.js';
import * as car from './dag.js';
import * as content from './content.js';
import * as mfs from './mfs.js';
import * as names from './names.js';
import * as pin from './pin.js';
import * as routing from './routing.js';

/** One executor per exported tool, keyed by contract tool name. */
export const EXECUTORS: Readonly<Record<string, Executor>> = Object.freeze({
  ...content,
  ...pin,
  ...mfs,
  ...names,
  ...car,
  ...routing,
});

/**
 * Checks that need only the input text. They run before the confirmation hook, so a person
 * is never asked to approve a call that would be refused anyway.
 */
export const PRECHECKS: Readonly<Record<string, (input: Record<string, any>) => void>> = Object.freeze({
  dag_put: (input) => {
    let parsed: unknown;
    try {
      parsed = JSON.parse(input.data);
    } catch {
      throw new RangeError('data is not valid JSON');
    }
    try {
      fromDagJson(parsed);
    } catch (e) {
      throw new RangeError(e instanceof RangeError ? e.message : 'data is not valid dag-json');
    }
  },
});
