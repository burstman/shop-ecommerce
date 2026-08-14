-- +goose Up
create table if not exists users(
	id SERIAL primary key,
	email text unique not null,
	password_hash text not null,
	first_name text not null,
	last_name text not null,
	role text not null,
	email_verified_at timestamp,
	created_at timestamp not null,
	updated_at timestamp not null,
	deleted_at timestamp
);

-- +goose Down
drop table if exists users;
