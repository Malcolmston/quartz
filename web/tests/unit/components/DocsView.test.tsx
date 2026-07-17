import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { DocsView } from '../../../src/components/DocsView';
import type { DocIndex } from 'go-ui';

// A minimal DocIndex the stubbed fetch returns for DocsApp's doc.json request.
const DOC_INDEX: DocIndex = {
  module: 'github.com/malcolmston/quartz',
  packages: [
    {
      importPath: 'github.com/malcolmston/quartz',
      name: 'quartz',
      synopsis: 'Package quartz is a standard-library-only Go job scheduler.',
      doc: 'Package quartz is a standard-library-only Go job scheduler.',
      consts: [],
      vars: [],
      types: [
        {
          name: 'Scheduler',
          signature: 'type Scheduler struct{}',
          doc: 'Scheduler fires triggers on a worker pool.',
          consts: [],
          vars: [],
          funcs: [],
          methods: [],
        },
      ],
      funcs: [{ name: 'NewScheduler', signature: 'func NewScheduler(opts Options) *Scheduler', doc: 'NewScheduler constructs a Scheduler.' }],
    },
  ],
};

describe('DocsView', () => {
  beforeEach(() => {
    // DocsApp fetches doc.json; return the small index.
    global.fetch = vi.fn((input: RequestInfo | URL) => {
      if (String(input).includes('doc.json')) {
        return Promise.resolve({ ok: true, json: () => Promise.resolve(DOC_INDEX) } as Response);
      }
      return new Promise<Response>(() => {});
    }) as unknown as typeof fetch;
  });

  it('renders the inline React API reference from the fetched doc.json', async () => {
    const { container } = render(<DocsView />);
    expect(container.querySelector('#view-docs')).not.toBeNull();
    expect(
      screen.getByRole('heading', { level: 2, name: /API documentation/ }),
    ).toBeInTheDocument();

    // DocsApp fetches asynchronously, then renders the package view + symbols.
    expect(await screen.findByRole('heading', { name: /package quartz/ })).toBeInTheDocument();
    expect(container.querySelector('#sym-NewScheduler'), 'func NewScheduler symbol card').not.toBeNull();
    expect(container.querySelector('#sym-Scheduler'), 'type Scheduler symbol card').not.toBeNull();

    // The secondary link to the raw generated static HTML remains.
    expect(screen.getByRole('link', { name: /Open the raw generated HTML/ })).toHaveAttribute('href', './api/');
  });
});
