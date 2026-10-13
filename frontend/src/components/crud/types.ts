export type CrudField = {
  name: string;
  label: string;
  type?: 'text' | 'email' | 'password' | 'number' | 'checkbox' | 'select';
  required?: boolean;
  placeholder?: string;
  options?: { value: string | number; label: string }[];
  help?: string;
};
export type CrudRecord = Record<string, unknown> & { id: number };
export type CrudDefinition = {
  title: string;
  description: string;
  singular: string;
  permission: string;
  createPermission: string;
  updatePermission: string;
  deletePermission?: string;
  removeLabel?: string;
  fields: CrudField[];
  list: () => Promise<CrudRecord[]>;
  create: (data: Record<string, unknown>) => Promise<unknown>;
  update: (id: number, data: Record<string, unknown>) => Promise<unknown>;
  remove?: (id: number) => Promise<unknown>;
  columns: { key: string; label: string; format?: (row: CrudRecord) => string }[];
  rowLabel: (row: CrudRecord) => string;
};
