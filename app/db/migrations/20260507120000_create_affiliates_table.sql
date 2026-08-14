-- +goose Up
create table if not exists affiliates(
	id SERIAL primary key,
	affiliate_id text unique not null,
	name text not null,
	email text,
	password_hash text not null,
	rate numeric(5,2) not null default 0,
	active boolean not null default true,
	created_at timestamp not null,
	updated_at timestamp not null,
	deleted_at timestamp
);

-- +goose Down
drop table if exists affiliates;
