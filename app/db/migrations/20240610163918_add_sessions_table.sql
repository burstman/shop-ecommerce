-- +goose Up
create table if not exists sessions(
	id SERIAL primary key,
	token text not null,
	user_id integer not null references users,
	ip_address text,
	user_agent text,
	expires_at timestamp not null, 
	created_at timestamp not null, 
    updated_at timestamp not null, 
	deleted_at timestamp 
);

-- +goose Down
drop table if exists sessions;
