create table workitems (
    id serial primary key,
    userid integer not null,
    workdatetime timestamp without time zone not null,
    timetype varchar(255) not null
);
