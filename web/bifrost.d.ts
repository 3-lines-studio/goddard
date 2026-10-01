declare module 'virtual:bifrost/routes' {
  export interface BifrostRoute {
    pattern: string;
    view: string;
    kind: 'server' | 'static' | 'client';
  }

  export const routes: BifrostRoute[];

  export function href(pattern: string, params?: Record<string, string | string[]>): string;
}

declare module 'virtual:bifrost/navigation' {
  export function navigate(href: string): Promise<void>;
  export function replace(href: string): Promise<void>;
  export function refresh(): Promise<void>;
  export function Link(props: { href: string; children?: unknown } & Record<string, unknown>): any;
  export function usePathname(): string;
  export function useParams(): Record<string, string | string[]>;
  export function useSearchParams(): URLSearchParams;
  export function useRouter(): {
    push(href: string): Promise<void>;
    replace(href: string): Promise<void>;
    refresh(): Promise<void>;
    back(): void;
    forward(): void;
  };
}
